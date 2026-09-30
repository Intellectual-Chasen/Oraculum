package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// 調査の段階の操作の pattern。
const (
	stagesPattern          = "GET /api/v0/stages"
	loadingStartPattern    = "POST /api/v0/stages/loading"
	processingStartPattern = "POST /api/v0/stages/processing"
	sourceFilesPattern     = "GET /api/v0/stages/source-files"
	// apiPathPrefix は、段階を終えるまで stage_not_ready で答える経路の path の接頭辞である。
	apiPathPrefix = "/api/v0/"
)

// Stages は、収集元の読み込みと処理の 2 つの段階を始め、その結果を返す port である。
//
// 本番は pipeline.Stages が実装する。test は処理を終えた段階を偽の実装で渡す。
type Stages interface {
	Snapshot() core.InvestigationStages
	StartRequestedLoading(plans []pipeline.SourcePlan) error
	StartRequestedLoadingThenProcessing(plans []pipeline.SourcePlan) error
	StartProcessing() error
	ListRequestedFiles(ctx context.Context, dir string, recursive bool) (core.SourceFileListing, error)
	Loaded() (pipeline.InvestigationStore, bool)
	Processed() (pipeline.Session, bool)
}

// NewStagedHandler は、段階の操作と、段階を終えた後の API を 1 つの handler にまとめる。
//
// **段階を終えるまでの要求を 409 stage_not_ready で退ける。** 空の応答を 200 で返すと、
// 「関係が無い」と区別できない。収集元の一覧 (`/api/v0/sources`) は読み込みを終えた時点で答え、それ
// 以外の API は処理を終えた時点で答える。
//
// **段階を終える前は、`/api/v0/` で始まる path をすべて段階の失敗で答える。** 経路の一覧を
// 別に持つと、経路を足したときに一覧が通知なしに古くなる。
//
// sigma は、読み込みを終えた取り込み結果に Sigma のルールを当てた結果を返す。処理を終えた後の
// 最初の要求で 1 回だけ読む。nil のときはルールの集合を持たない結果を返す。
func NewStagedHandler(
	stages Stages, sigma func() pipeline.SigmaEvaluation, rules pipeline.AttackRuleSet,
) (http.Handler, error) {
	handler := &stagedHandler{stages: stages, rules: rules, sigma: sigma}
	mux := http.NewServeMux()
	mux.HandleFunc(stagesPattern, handler.snapshot)
	mux.HandleFunc(loadingStartPattern, handler.startLoading)
	mux.HandleFunc(processingStartPattern, handler.startProcessing)
	mux.HandleFunc(sourceFilesPattern, handler.sourceFiles)
	mux.HandleFunc(sourceUploadPattern, handler.uploadSource)
	mux.HandleFunc("/", handler.investigation)
	return mux, nil
}

// stagedHandler は段階の操作と、段階を終えた後の API を振り分ける。
type stagedHandler struct {
	stages Stages
	rules  pipeline.AttackRuleSet
	sigma  func() pipeline.SigmaEvaluation
	// mu は loaded と processed の組み立てを、それぞれ 1 回にする。
	mu sync.Mutex
	// loaded は処理を終える前の収集元の一覧である (loadedSources)。processed を組んだ時点で nil に戻す。
	loaded    http.Handler
	processed http.Handler
}

// snapshot は段階の状態と進行を返す。
func (h *stagedHandler) snapshot(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Query()) != 0 {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: "the operation takes no query parameter",
		})
		return
	}
	h.writeSnapshot(w, http.StatusOK)
}

// writeSnapshot は段階の状態を検査して書く。
func (h *stagedHandler) writeSnapshot(w http.ResponseWriter, status int) {
	snapshot := h.stages.Snapshot()
	if err := snapshot.Validate(); err != nil {
		slog.Error("the stages broke their invariants", "error", err)
		writeError(w, http.StatusInternalServerError, core.ApiError{
			Code: core.ApiErrorCodeInternalError, Message: "reading the stages failed",
		})
		return
	}
	writeJSON(w, status, snapshot)
}

// loadingRequestBody は読み込みを始める要求の本文である。**本型が項目の定義元である。**
type loadingRequestBody struct {
	Sources []loadingRequestSource `json:"sources"`
	// ThenProcess は、読み込みが完了したら続けて処理を始めるかである。
	ThenProcess bool `json:"thenProcess,omitempty"`
}

// loadingRequestSource は読み込む収集元 1 件の指定である。項目の名前は取り込みの記録
// (sourceargs.ImportSpecSource) と揃える。
type loadingRequestSource struct {
	// OriginPath は server が決めた基準の directory からの相対 path である。
	OriginPath string         `json:"originPath"`
	FormatKey  core.FormatKey `json:"formatKey"`
	FormatSpec *string        `json:"formatSpec,omitempty"`
	CaseId     *string        `json:"caseId,omitempty"`
	// Terminal は収集元を記録した端末である。
	Terminal *loadingRequestTerminal `json:"terminal,omitempty"`
}

// loadingRequestTerminal は収集元に指定する端末である。3 項目のうち 1 つ以上を持つ。
type loadingRequestTerminal struct {
	Id       string `json:"id,omitempty"`
	Hostname string `json:"hostname,omitempty"`
	Ip       string `json:"ip,omitempty"`
}

// plans は要求を取り込みの計画にする。値の検査は pipeline.Stages が行う。
func (b loadingRequestBody) plans() []pipeline.SourcePlan {
	plans := make([]pipeline.SourcePlan, len(b.Sources))
	for index, source := range b.Sources {
		// path は要求の文字列のまま渡す。退けた収集元を、分析者が入力した文字列で指す。
		plans[index] = pipeline.SourcePlan{
			OriginPath: source.OriginPath, FormatKey: source.FormatKey,
			FormatSpec: source.FormatSpec, CaseId: source.CaseId,
		}
		if source.Terminal != nil {
			plans[index].Terminal = &pipeline.SourceTerminal{
				TerminalId: source.Terminal.Id, TerminalHostname: source.Terminal.Hostname,
				Ip: source.Terminal.Ip,
			}
		}
	}
	return plans
}

// startLoading は、要求が指す収集元の読み込みを背景で始める。
func (h *stagedHandler) startLoading(w http.ResponseWriter, r *http.Request) {
	body, apiError := decodeLoadingRequestBody(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	start := h.stages.StartRequestedLoading
	if body.ThenProcess {
		start = h.stages.StartRequestedLoadingThenProcessing
	}
	if err := start(body.plans()); err != nil {
		writeStageError(w, err)
		return
	}
	h.writeSnapshot(w, http.StatusAccepted)
}

// sourceFiles は、基準の directory の下の directory 1 つの項目を、file ごとの入力形式の候補と
// ともに返す。query の path は基準の directory からの相対 path で、省略すると基準の directory
// そのものである。recursive が true のときは下の directory の file も返す。
func (h *stagedHandler) sourceFiles(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	invalid := func(message string) {
		writeError(w, http.StatusBadRequest, core.ApiError{Code: core.ApiErrorCodeInvalidRequest, Message: message})
	}
	for name, values := range query {
		if (name != "path" && name != "recursive") || len(values) != 1 {
			invalid("the operation takes the query parameters path and recursive at most once each")
			return
		}
	}
	dir := "."
	if query.Has("path") {
		dir = query.Get("path")
	}
	recursive := false
	switch query.Get("recursive") {
	case "", "false":
	case "true":
		recursive = true
	default:
		invalid("the query parameter recursive takes true or false")
		return
	}
	listing, err := h.stages.ListRequestedFiles(r.Context(), dir, recursive)
	// 要求を送った画面が接続を閉じた。応答を読む相手がいない。
	if r.Context().Err() != nil {
		return
	}
	if err != nil {
		writeStageError(w, err)
		return
	}
	if err := listing.Validate(); err != nil {
		slog.Error("the listing of the base directory broke its invariants", "error", err)
		writeError(w, http.StatusInternalServerError, core.ApiError{
			Code: core.ApiErrorCodeInternalError, Message: "reading the base directory failed",
		})
		return
	}
	writeJSON(w, http.StatusOK, listing)
}

// startProcessing は処理を背景で始める。本文を取らない。
func (h *stagedHandler) startProcessing(w http.ResponseWriter, r *http.Request) {
	if content, err := io.ReadAll(io.LimitReader(r.Body, requestBodyLimit)); err != nil ||
		strings.TrimSpace(string(content)) != "" {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: "the operation takes no request body",
		})
		return
	}
	if err := h.stages.StartProcessing(); err != nil {
		writeStageError(w, err)
		return
	}
	h.writeSnapshot(w, http.StatusAccepted)
}

// writeStageError は段階の操作の失敗を status に写す。
//
// **要求起因の失敗だけを個別に 4xx にする。** それ以外は内部の破れとして 500 にし、理由を
// 応答に載せない。
func writeStageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pipeline.ErrLoadingRequestInvalid):
		// 要求の file を確かめた失敗の原因は、運用者への log だけが持つ。
		slog.Warn("refused the loading request", "error", err)
		apiError := core.ApiError{Code: core.ApiErrorCodeInvalidRequest, Message: "the loading request is invalid"}
		// 退けた理由と、退けた収集元を分析者が入力した path で指す。**応答の文字列は、基準の
		// directory の path と OS の失敗の文字列を持たない** (pipeline.LoadingRequestError.Message)。
		if requestError, ok := errors.AsType[*pipeline.LoadingRequestError](err); ok {
			apiError.Message = requestError.Message()
			apiError.LoadingRejection = requestError.Rejection
			apiError.OriginPath = requestError.OriginPath
		}
		writeError(w, http.StatusBadRequest, apiError)
	case errors.Is(err, pipeline.ErrStageNotReady):
		writeError(w, http.StatusConflict, core.ApiError{
			Code: core.ApiErrorCodeStageNotReady, Message: "the loading of the sources has not completed",
		})
	case errors.Is(err, pipeline.ErrStageAlreadyStarted):
		writeError(w, http.StatusConflict, core.ApiError{
			Code: core.ApiErrorCodeStageAlreadyStarted, Message: "the stage is running or has completed",
		})
	default:
		slog.Error("starting the stage failed", "error", err)
		writeError(w, http.StatusInternalServerError, core.ApiError{
			Code: core.ApiErrorCodeInternalError, Message: "starting the stage failed",
		})
	}
}

// decodeLoadingRequestBody は要求の本文を読む。未知の項目と、後ろに続く値を持つ本文を退ける。
func decodeLoadingRequestBody(r *http.Request) (loadingRequestBody, *core.ApiError) {
	invalid := func(message string) *core.ApiError {
		return &core.ApiError{Code: core.ApiErrorCodeInvalidRequest, Message: message}
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, requestBodyLimit))
	decoder.DisallowUnknownFields()
	var body loadingRequestBody
	if err := decoder.Decode(&body); err != nil {
		return loadingRequestBody{}, invalid("the request body is not a readable loading request")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return loadingRequestBody{}, invalid("the request body carries a value after its JSON object")
	}
	return body, nil
}

// investigation は段階を終えた後の API へ要求を渡す。段階を終えていない要求を退ける。
func (h *stagedHandler) investigation(w http.ResponseWriter, r *http.Request) {
	if processed, ready := h.processedHandler(); ready {
		processed.ServeHTTP(w, r)
		return
	}
	if !strings.HasPrefix(r.URL.Path, apiPathPrefix) {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == strings.TrimPrefix(sourcesPattern, "GET ") {
		if sources, loaded := h.loadedSources(); loaded {
			sources.ServeHTTP(w, r)
			return
		}
	}
	writeError(w, http.StatusConflict, core.ApiError{
		Code:    core.ApiErrorCodeStageNotReady,
		Message: "the operation needs the loading and the processing of the sources to complete",
	})
}

// loadedSources は、処理を終える前の収集元の一覧に答える handler を返す。ok が偽になるのは、
// 読み込みを終えていないときである。
//
// **端末の割当と収集元の時刻の解釈を当てた取り込み結果で答える。** 処理を終えた後の一覧と同じ
// 項目を答える。解釈を記録した調査を開き直した直後に、解釈で読んだ観測期間を除かない。
//
// **組むのは読み込みを終えた後の最初の要求の 1 回だけである。** 読み込みを終えた保存先は
// 差し替わらず (pipeline.Stages)、処理を終えるまでは分析者の入力を記録する経路が答えない。
func (h *stagedHandler) loadedSources() (http.Handler, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// 処理の API を確かめた後に、別の要求が処理の API を組んだ場合である。処理の API も収集元の
	// 一覧に答える。
	if h.processed != nil {
		return h.processed, true
	}
	if h.loaded != nil {
		return h.loaded, true
	}
	store, loaded := h.stages.Loaded()
	if !loaded {
		return nil, false
	}
	h.loaded = sourcesHandler{result: pipeline.CurrentImportResult(store)}
	return h.loaded, true
}

// processedHandler は処理を終えた調査の API を返す。**組むのは処理を終えた後の最初の要求の 1 回
// だけである。** 処理を終えた Session は差し替わらない (pipeline.Stages)。
//
// **組んだ時点で、処理の前の収集元の一覧を手放す。** 処理を終えた後の要求はすべてこの API が
// 答える。時刻の解釈を当てた取り込み結果はレコードの複製を持つ。
func (h *stagedHandler) processedHandler() (http.Handler, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.processed != nil {
		return h.processed, true
	}
	session, ready := h.stages.Processed()
	if !ready {
		return nil, false
	}
	processed := newInvestigationHandler(session.Store, session.Catalog, pipeline.SigmaEvaluation{}, h.rules)
	if h.sigma != nil {
		processed.sigma = h.sigma()
	}
	h.processed = processed
	h.loaded = nil
	return h.processed, true
}
