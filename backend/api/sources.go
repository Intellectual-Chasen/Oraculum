package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"sync"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const sourcesPattern = "GET /api/v0/sources"

// NewHandler は取り込み結果と分析者の所見を参照する API handler を返す。
//
// **保存先は port で受け取る。** api は取り込み結果を直に握らず、保存先の実装を
// 差し替えても本 package を書き換えずに済む形にする。
//
// **グラフは catalog が組む。** 組み立ての間も、グラフを読まない要求は待たない。
// レコードを位置で探す索引と、応答の fields を組む器は、分析者が与えた端末の割当が
// 変わった後の最初の要求でだけ組み直す。組み直すたびに取り込み結果の全レコードを走査する
// ためである (pipeline.NewCandidateIndex と pipeline.NewFieldsBuilder)。
//
// sigma は起動時に Sigma のルールを当てた結果である。レコードは端末の割当で変わらないため、
// 組み直さない。
func NewHandler(
	store pipeline.InvestigationStore, catalog *pipeline.GraphCatalog, sigma pipeline.SigmaEvaluation,
	rules pipeline.AttackRuleSet,
) (http.Handler, error) {
	return newInvestigationHandler(store, catalog, sigma, rules), nil
}

// newInvestigationHandler は、組み終えた catalog と保存先から調査の API を組む。
func newInvestigationHandler(
	store pipeline.InvestigationStore, catalog *pipeline.GraphCatalog, sigma pipeline.SigmaEvaluation,
	rules pipeline.AttackRuleSet,
) *investigationHandler {
	return &investigationHandler{store: store, catalog: catalog, revision: -1, rules: rules, sigma: sigma}
}

// WithCandidateOriginLog は、layers が関連付けの候補のエッジを足すたびに、起点の分類を運用者へ
// 出す組み方を返す。
//
// **log を出すのは出力境界である本 package とする。** 宣言の欠陥の文字列を無害化してから出す。
func WithCandidateOriginLog(layers pipeline.GraphLayers) pipeline.GraphLayers {
	withCandidates := layers.WithCandidates
	layers.WithCandidates = func(
		observed pipeline.Graph, result pipeline.ImportResult, selection pipeline.MatchConditionSelection,
	) pipeline.Graph {
		graph := withCandidates(observed, result, selection)
		logCandidateOrigins(graph)
		return graph
	}
	return layers
}

// investigationHandler は、分析者が与えた端末の割当が変わったときに索引を組み直す。
//
// **割当は端末の判定の材料である。** 割当を 1 件足すと、どのレコードがどの端末のものかが
// 変わり、索引とグラフと応答の fields が変わる。起動時の 1 回だけ組む形では、分析者が
// 与えた割当が画面に出ない。
//
// 組み直すのは割当が変わった後の最初の要求だけである。取り込み結果は変わらないため、
// 同じ割当の間は組んだ器を使い回す。
type investigationHandler struct {
	store   pipeline.InvestigationStore
	catalog *pipeline.GraphCatalog
	rules   pipeline.AttackRuleSet
	sigma   pipeline.SigmaEvaluation
	// mu が守るのは経路の組の差し替えだけである。グラフの組み立ては catalog が lock の外で行う。
	mu sync.Mutex
	// revision は組んだ器が見ていた割当の番号である。-1 は 1 度も組んでいない状態である。
	revision int64
	routes   http.Handler
}

func (h *investigationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.current().ServeHTTP(w, r)
}

// current は現在の割当に対応する経路の組を返す。
func (h *investigationHandler) current() http.Handler {
	result, revision := h.catalog.Current()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.routes != nil && h.revision == revision {
		return h.routes
	}
	h.routes = buildRoutes(h.store, result, h.catalog, h.rules, h.sigma)
	h.revision = revision
	return h.routes
}

// graphSource は、要求が与えた関連付けの条件に対応するグラフと取り込み結果を返す関数である。
type graphSource func(
	context.Context, pipeline.MatchConditionSelection,
) (pipeline.Graph, pipeline.ImportResult, error)

// buildRoutes は取り込み結果と、読み出した時点の割当から、要求に答える経路の組を作る。
//
// **グラフを読む経路だけが関連付けの条件を読む。** 取り込み状況・端末の割当は関連付けの条件で
// 変わらないため、これらの要求に条件を求めない。原資料のレコードは、起点を与えて到達した経路を
// 求める要求だけが条件を読む。
func buildRoutes(
	store pipeline.InvestigationStore, result pipeline.ImportResult, catalog *pipeline.GraphCatalog,
	rules pipeline.AttackRuleSet, sigma pipeline.SigmaEvaluation,
) http.Handler {
	graphs := graphSource(catalog.Graph)
	index := pipeline.NewCandidateIndex(result)
	mux := http.NewServeMux()
	// method を pattern に含めるため、GET 以外の要求には ServeMux が 405 と Allow header を返す。
	mux.Handle(sourcesPattern, sourcesHandler{result: result})
	mux.Handle(recordNumbersPattern, recordNumbersHandler{result: result})
	mux.Handle(rawTextsPattern, rawTextsHandler{result: result})
	fields := pipeline.NewFieldsBuilder(result)
	records := recordsHandler{
		result: result, index: index, fields: fields, graphs: graphs,
	}
	mux.Handle(recordsPattern, records)
	selecting := func(
		build func(pipeline.Graph, pipeline.ImportResult) http.Handler,
	) http.Handler {
		return matchSelectingHandler{graphs: graphs, build: build}
	}
	mux.Handle(recordGraphPattern, selecting(
		func(g pipeline.Graph, _ pipeline.ImportResult) http.Handler {
			return recordGraphHandler{records: records, graph: g}
		}))
	mux.Handle(influencePathPattern, selecting(
		func(g pipeline.Graph, _ pipeline.ImportResult) http.Handler {
			return influencePathHandler{graph: g}
		}))
	mux.Handle(graphPattern, selecting(
		func(g pipeline.Graph, result pipeline.ImportResult) http.Handler {
			return graphHandler{result: result, graph: g}
		}))
	mux.Handle(sigmaRuleCandidatesPattern, selecting(
		func(g pipeline.Graph, _ pipeline.ImportResult) http.Handler {
			return sigmaRuleCandidatesHandler{graph: g, evaluation: sigma}
		}))
	mux.Handle(attackCandidatesPattern, selecting(
		func(g pipeline.Graph, _ pipeline.ImportResult) http.Handler {
			return attackCandidatesHandler{graph: g, ruleSet: rules}
		}))
	mux.Handle(investigationOrderPattern, selecting(
		func(g pipeline.Graph, _ pipeline.ImportResult) http.Handler {
			return investigationOrderHandler{graph: g, sigma: sigma}
		}))
	// **時系列は観測の層で返す。** 時系列の行は関連付けの候補のエッジに依らないため、選択ごとの
	// 組み立てを待たない。条件の検査は他のグラフを読む経路と同じく行う。
	observed := func(
		ctx context.Context, _ pipeline.MatchConditionSelection,
	) (pipeline.Graph, pipeline.ImportResult, error) {
		return catalog.Observed(ctx)
	}
	mux.Handle(timelinePattern, matchSelectingHandler{
		graphs: observed,
		build: func(g pipeline.Graph, result pipeline.ImportResult) http.Handler {
			return timelineHandler{graph: g, result: result}
		},
	})
	mux.Handle(accountRelationsPattern, matchSelectingHandler{
		graphs: observed,
		build: func(g pipeline.Graph, _ pipeline.ImportResult) http.Handler {
			return accountRelationsHandler{graph: g, sigma: sigma}
		},
	})
	// ノードの一覧は観測したノードとエッジの根拠だけを数えるため、時系列と同じく観測の層を読む。
	mux.Handle(nodeSummariesPattern, matchSelectingHandler{
		graphs: observed,
		build: func(g pipeline.Graph, _ pipeline.ImportResult) http.Handler {
			return nodeSummariesHandler{graph: g}
		},
	})
	// 件数の分布は時系列と同じレコードを数えるため、時系列と同じく観測の層を読む。
	mux.Handle(timeHistogramPattern, matchSelectingHandler{
		graphs: observed,
		build: func(g pipeline.Graph, _ pipeline.ImportResult) http.Handler {
			return timeHistogramHandler{graph: g}
		},
	})
	// 事象の種別もレコードの欄だけから数えるため、時系列と同じく観測の層を読む。
	mux.Handle(eventKindsPattern, matchSelectingHandler{
		graphs: observed,
		build: func(g pipeline.Graph, result pipeline.ImportResult) http.Handler {
			return eventKindsHandler{graph: g, result: result}
		},
	})
	// Proxy を経由しない接続もレコードの欄と端末の割当だけから数えるため、観測の層を読む。
	mux.Handle(proxyBypassPattern, matchSelectingHandler{
		graphs: observed,
		build: func(_ pipeline.Graph, result pipeline.ImportResult) http.Handler {
			return proxyBypassHandler{result: result}
		},
	})
	// 端末の情報とレコードの分類は観測の層で組むため、観測の層を読む。
	observing := func(build func(pipeline.Graph) http.Handler) http.Handler {
		return matchSelectingHandler{graphs: observed, build: func(g pipeline.Graph, _ pipeline.ImportResult) http.Handler {
			return build(g)
		}}
	}
	mux.Handle(terminalsPattern, observing(func(g pipeline.Graph) http.Handler { return terminalsHandler{graph: g} }))
	mux.Handle(terminalPattern, observing(func(g pipeline.Graph) http.Handler { return terminalHandler{graph: g} }))
	mux.Handle(terminalEventsPattern, observing(func(g pipeline.Graph) http.Handler { return terminalEventsHandler{graph: g} }))
	mux.Handle(nodesPattern, selecting(
		func(g pipeline.Graph, result pipeline.ImportResult) http.Handler {
			return nodesHandler{result: result, graph: g}
		}))
	// 関係の相手側の欄は、選択ごとのグラフの候補のエッジも辿る。
	mux.Handle(nodeValueCountsPattern, selecting(
		func(g pipeline.Graph, _ pipeline.ImportResult) http.Handler {
			return nodeValueCountsHandler{graph: g}
		}))
	mux.Handle(edgesPattern, selecting(
		func(g pipeline.Graph, result pipeline.ImportResult) http.Handler {
			return edgesHandler{result: result, graph: g}
		}))
	// URL の断片は観測した関係の根拠だけから組むため、時系列と同じく観測の層を読む。
	mux.Handle(edgeUrlFragmentsPattern, matchSelectingHandler{
		graphs: observed,
		build: func(g pipeline.Graph, _ pipeline.ImportResult) http.Handler {
			return edgeUrlFragmentsHandler{graph: g}
		},
	})
	assertions := selecting(
		func(g pipeline.Graph, _ pipeline.ImportResult) http.Handler {
			return assertionsHandler{
				store: store.Assertions(), resolver: pipeline.NewAssertionResolver(g),
				timeInterpreted: catalog.Invalidate,
			}
		})
	mux.Handle(assertionsPattern, assertions)
	mux.Handle(assertionCreatePattern, assertions)
	mux.Handle(assertionRevisionPattern, assertions)
	registerAssistProposals(mux, store, graphs)
	terminalAssignments := terminalAssignmentsHandler{
		result: result, store: store.TerminalAssignments(), recorded: catalog.Invalidate,
	}
	mux.Handle(terminalAssignmentsPattern, terminalAssignments)
	mux.Handle(terminalAssignmentCreatePattern, terminalAssignments)
	assistPermissions := assistPermissionsHandler{store: store.Assist()}
	mux.Handle(assistPermissionsPattern, assistPermissions)
	mux.Handle(assistPermissionRecordPattern, assistPermissions)
	registerAssistConversations(mux, assistConversationsHandler{
		store: store.Assist(), investigation: store, result: result, index: index, fields: fields,
		graphs: graphs, observed: observed,
	})
	return mux
}

// logCandidateOrigins は関連付けの起点を分類ごとに数えた件数を 1 行で出す。
//
// **候補を挙げなかった起点を通知せずに捨てない。** 「関連付けが候補を挙げなかった」と「関連付けを
// そもそも実行できなかった」を運用者が分けて読める経路である。出すのは件数だけで、
// 原資料の文字列を載せない。
func logCandidateOrigins(graph pipeline.Graph) {
	counts := graph.CandidateOriginCounts()
	attributes := make([]any, 0, len(counts)+3)
	total := 0
	for _, count := range counts {
		attributes = append(attributes, slog.Int(string(count.Outcome), count.OriginCount))
		total += count.OriginCount
	}
	// 関連付けの起点を 1 件も持たない取り込みでは、分類ごとの 0 件だけの行を出さない。
	if total == 0 {
		return
	}
	// **除いた候補は起点の分類に出ない。** 5 件のうち 3 件だけを除いた起点も matched に
	// なるため、候補 1 件ごとの漏れを別の項目で出す。
	records := graph.CandidateRecordCounts()
	attributes = append(attributes,
		slog.Int("unreadable_candidates", records.Unreadable),
		slog.Int("dropped_candidates", records.Dropped),
		// **レコードのノードを組めなかった起点は、分類の合計にも入らない。** その起点の
		// 結果はノードの詳細からも読めないため、この行が唯一の観測の経路である。
		slog.Int("origins_without_record_node", graph.OriginsWithoutRecordNode()))
	// 要素の名前は文字列の literal か core.RelationDerivationOutcome である。後者の値は
	// CandidateOriginCounts が candidateOriginOutcomes の一覧を走査して置くため、契約が
	// 定める閉じた集合に収まる。値はすべて int である。原資料の byte はこの行に入らない。
	// #nosec G706 -- 要素の名前も値も外部由来の文字列を持たない。
	slog.Info("counted the matching origins by outcome", attributes...)
	// **宣言の欠陥を件数だけで終わらせない。** グラフの経路は応答を 500 にせず候補の
	// エッジが消えるだけであるため、原因の宣言へ到達できる文字列を別の行で出す。
	// **候補集合が退けた理由を件数だけで終わらせない。** どの検査が退けたかへ、分類の
	// 件数からは到達できない。
	if problem := graph.CandidateSetProblem(); problem != nil {
		// #nosec G706 -- output.Sanitize が制御文字と改行を除いてから log へ渡す。
		slog.Error("building the candidate set failed",
			slog.String("candidate_set_problem", output.Sanitize(problem.Error())))
	}
	if problem := graph.SourceDeclarationProblem(); problem != nil {
		// 宣言の文字列は入力形式と収集元から来る。制御文字と改行を通すと、1 件の log が
		// 複数行に割れる。
		// #nosec G706 -- output.Sanitize が制御文字と改行を除いてから log へ渡す。
		slog.Error("composing the stage conditions failed",
			slog.String("declaration_problem", output.Sanitize(problem.Error())))
	}
}

type sourcesHandler struct {
	result pipeline.ImportResult
}

// sourcesResponse は取り込んだ収集元を全件返す。上限も続きを取る位置も持たない。
type sourcesResponse struct {
	Sources     []sourceResponseItem `json:"sources"`
	SourceCount int64                `json:"sourceCount"`
	EmptyReason core.EmptyReason     `json:"emptyReason,omitempty"`
	// SkippedFiles は、収集の directory にあり取り込まなかった file である。収集の directory を
	// 指定しなかった取り込みでは要素数 0 である。
	SkippedFiles []core.SkippedFile `json:"skippedFiles"`
}

type sourceResponseItem struct {
	Source       core.SourceIdentity `json:"source"`
	ImportStatus core.ImportStatus   `json:"importStatus"`
	// InterpretedObservedRange は、観測期間を持たない収集元を分析者の時刻の解釈で読んだ観測
	// 期間である。両端は地方時の文字列と分析者のずれ (interpretation) の組である。解釈を持たない
	// 収集元と、観測期間を持つ収集元では出ない。端末の割当は、ずれを外した地方時の期間で
	// 記録する。
	// **収集元の識別の観測期間を書き換えない。** 原資料から定まる値と分析者の解釈を分けて持つ
	// (pipeline.ImportResult.InterpretedObservedRange)。
	InterpretedObservedRange *core.TimeRange `json:"interpretedObservedRange,omitempty"`
	// ImportTimeOffset は、利用者が取り込みの起動で収集元に指定した UTC からのずれである。
	// 分析者が解釈を記録していない収集元の地方時の時刻は、このずれで読む。分析者が解釈を記録
	// すると、時刻は記録した解釈で読み、本項目は起動で指定した値のまま出る。ずれを指定して
	// いない収集元では出ない。
	ImportTimeOffset core.UtcOffset `json:"importTimeOffset,omitempty"`
	// MessageUnrenderedRecordRefs は、説明を組めなかったレコードの位置である。収集元の中の順に
	// 並び、先頭の core.RecordNumberListLimit 件までを持つ。件数の総数は
	// source.messageUnrenderedCount である。該当するレコードが無い収集元では出ない。
	MessageUnrenderedRecordRefs []core.RecordLocator `json:"messageUnrenderedRecordRefs,omitempty"`
}

func (h sourcesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := checkSourcesParameterNames(r.URL.Query()); err != nil {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code:    core.ApiErrorCodeInvalidRequest,
			Message: err.Error(),
		})
		return
	}
	entries, err := h.result.SourceEntries()
	if err != nil {
		// 内部不変条件の破れであり、応答は理由を持たない。原因を追えるのはこの記録だけで
		// ある。err は収集元の識別子だけを持ち、原資料の byte 列を持たない。
		slog.Error("listing sources failed", "error", err)
		writeError(w, http.StatusInternalServerError, core.ApiError{
			Code:    core.ApiErrorCodeInternalError,
			Message: "listing sources failed",
		})
		return
	}
	response := sourcesResponse{
		Sources:      make([]sourceResponseItem, 0, len(entries)),
		SourceCount:  int64(len(entries)),
		SkippedFiles: append([]core.SkippedFile{}, h.result.SkippedFiles()...),
	}
	for _, entry := range entries {
		item := sourceResponseItem{Source: entry.Identity, ImportStatus: entry.Status}
		if interpreted, found := h.result.InterpretedObservedRange(entry.Identity.SourceId); found {
			item.InterpretedObservedRange = &interpreted
		}
		item.ImportTimeOffset, _ = h.result.ImportTimeOffset(entry.Identity.SourceId)
		item.MessageUnrenderedRecordRefs, _ = h.result.MessageUnrenderedRecordRefs(
			entry.Identity.SourceId, core.RecordNumberListLimit)
		response.Sources = append(response.Sources, item)
	}
	if response.SourceCount == 0 {
		response.EmptyReason = core.EmptyReasonNoSourceIngested
	}
	writeJSON(w, http.StatusOK, response)
}

// checkSourcesParameterNames は要求の項目を確かめる。収集元の一覧は項目を 1 つも
// 読まないため、値を持つ要求をすべて退ける。
//
// 未知の項目は invalid_request の条件に入っていない。綴り誤りを通知せずに既定値で処理する
// 応答を避けるため拒否する。
func checkSourcesParameterNames(query url.Values) error {
	for name := range query {
		return errors.New(name + " is not a query parameter of a sources request")
	}
	return nil
}

// httpStatusFor は code に対応する HTTP status を返す。
// **本関数が code と status の対応の定義元である。** switch に無い code は internal_error と
// 同じ 500 にする。
func httpStatusFor(code core.ApiErrorCode) int {
	switch code {
	case core.ApiErrorCodeNotImplemented:
		return http.StatusNotImplemented
	case core.ApiErrorCodeCandidateWindowMissing, core.ApiErrorCodeInvalidRequest:
		return http.StatusBadRequest
	case core.ApiErrorCodeSourceHashMismatch, core.ApiErrorCodeImportWithheld,
		core.ApiErrorCodeStageNotReady, core.ApiErrorCodeStageAlreadyStarted,
		core.ApiErrorCodeTerminalAssignmentAlreadyRecorded, core.ApiErrorCodeSourceUploadAlreadyExists, core.ApiErrorCodeWorkspaceChanged,
		core.ApiErrorCodeAssertionChanged,
		core.ApiErrorCodeAssistUnavailable, core.ApiErrorCodeAssistNotPermitted,
		core.ApiErrorCodeAssistProposalDecided:
		return http.StatusConflict
	case core.ApiErrorCodeSourceNotFound, core.ApiErrorCodePositionOutsideSource,
		core.ApiErrorCodeRecordNotFound, core.ApiErrorCodeRecordUnreadable, core.ApiErrorCodeInvestigationNotFound,
		core.ApiErrorCodeConversationNotFound:
		return http.StatusNotFound
	case core.ApiErrorCodeRequestOriginRejected, core.ApiErrorCodePermissionDenied:
		return http.StatusForbidden
	case core.ApiErrorCodeAuthenticationRequired, core.ApiErrorCodeLoginRejected:
		return http.StatusUnauthorized
	case core.ApiErrorCodeLoginRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := output.WriteJSON(w, value); err != nil {
		// status line と header は送信済みで、応答の形を変える手段が残っていない。
		// 本体が途中で切れた事象を記録するだけにする。err は外部由来の文字列を持たない。
		slog.Error("writing API response body failed", "status", status, "error", err)
	}
}

// writeError は応答へ出る前に、外部由来の文字列を持ちうる項目をすべて無害化する。
// 外部由来の文字列は、出力境界で無害化してから log と画面に出す。SourceId と
// SourceContentSha256 と OriginPath と ImportStatusRef は要求が与える値を載せる操作が
// 後続で入る。RecordRef の SourceFileName は起動引数が
// 与える file 名を持つ。制御文字を含まない値は output.Sanitize を通しても変わらない。
func writeError(w http.ResponseWriter, status int, apiError core.ApiError) {
	apiError.Message = output.Sanitize(apiError.Message)
	apiError.SourceId = output.Sanitize(apiError.SourceId)
	apiError.SourceContentSha256 = output.Sanitize(apiError.SourceContentSha256)
	apiError.OriginPath = output.Sanitize(apiError.OriginPath)
	apiError.ImportStatusRef = output.Sanitize(apiError.ImportStatusRef)
	for i, name := range apiError.MissingParameters {
		apiError.MissingParameters[i] = output.Sanitize(name)
	}
	if apiError.RecordRef != nil {
		apiError.RecordRef = sanitizedLocator(*apiError.RecordRef)
	}
	writeJSON(w, status, apiError)
}

// sanitizedLocator はレコード位置の文字列の項目を無害化した複製を返す。
// 呼び出し元が渡した組を書き換えない。
func sanitizedLocator(locator core.RecordLocator) *core.RecordLocator {
	locator.SourceId = output.Sanitize(locator.SourceId)
	locator.SourceContentSha256 = output.Sanitize(locator.SourceContentSha256)
	locator.SourceFileName = output.Sanitize(locator.SourceFileName)
	locator.RecordRawTextRef = output.Sanitize(locator.RecordRawTextRef)
	return &locator
}
