package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// 会話の操作の pattern。
//
// **中継の tool だけが呼ぶ path である。** 中継は browser からの要求をこの path の下へ転送しない。
// 画面が使う受け渡しの記録の閲覧は、この path の外に置く。
const (
	conversationOpenPattern     = "POST /api/v0/conversations"
	conversationTurnPattern     = "POST /api/v0/conversations/{conversationId}/turns"
	conversationOverviewPattern = "POST /api/v0/conversations/{conversationId}/overview"
	conversationGraphPattern    = "POST /api/v0/conversations/{conversationId}/graph-search"
	conversationTargetPattern   = "POST /api/v0/conversations/{conversationId}/targets"
	conversationRecordsPattern  = "POST /api/v0/conversations/{conversationId}/records"
	conversationTimelinePattern = "POST /api/v0/conversations/{conversationId}/timeline"
	conversationCheckPattern    = "POST /api/v0/conversations/{conversationId}/search-query-checks"
	assistDisclosuresPattern    = "GET /api/v0/assist-disclosures"
	conversationIdPathValue     = "conversationId"
	conversationParam           = "conversation"
)

// assistConversationsHandler は、中継の tool が呼ぶ会話の操作に答え、受け渡しを監査記録に残す。
//
// **本文を返す前に受け渡しの記録を書く。** 書けなかった本文は返さない。許可は受け渡しのたびに
// 保存先が確かめ、会話の途中で取り消されたら以後の受け渡しを退ける。
type assistConversationsHandler struct {
	store         pipeline.AssistStore
	investigation pipeline.InvestigationStore
	result        pipeline.ImportResult
	index         pipeline.CandidateIndex
	fields        *pipeline.FieldsBuilder
	graphs        graphSource
	observed      graphSource
}

// registerAssistConversations は会話の操作と受け渡しの記録の閲覧を mux へ登録する。
func registerAssistConversations(mux *http.ServeMux, handler assistConversationsHandler) {
	guarded := func(serve func(http.ResponseWriter, *http.Request)) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !handler.store.Available() {
				writeError(w, http.StatusConflict, core.ApiError{
					Code:    core.ApiErrorCodeAssistUnavailable,
					Message: "the launch keeps no investigation, so it holds no assist conversation",
				})
				return
			}
			serve(w, r)
		})
	}
	mux.Handle(conversationOpenPattern, guarded(handler.open))
	mux.Handle(conversationTurnPattern, guarded(handler.registerTurn))
	mux.Handle(conversationOverviewPattern, guarded(handler.overview))
	mux.Handle(conversationGraphPattern, guarded(handler.graphSearch))
	mux.Handle(conversationTargetPattern, guarded(handler.target))
	mux.Handle(conversationRecordsPattern, guarded(handler.records))
	mux.Handle(conversationTimelinePattern, guarded(handler.timeline))
	mux.Handle(conversationCheckPattern, guarded(handler.checkSearchQuery))
	mux.Handle(assistDisclosuresPattern, guarded(handler.disclosures))
}

// conversationOpenBody は会話を発行する要求の本文である。
type conversationOpenBody struct {
	Provider core.AssistProvider `json:"provider"`
	// Model は中継が申告する提供者の model の名前である。
	Model string `json:"model,omitempty"`
}

// conversationOpenResponse は会話の発行の応答である。
type conversationOpenResponse struct {
	Conversation core.AssistConversation `json:"conversation"`
}

// open は、提供者への送信を現在の改訂が許可しているときに会話を発行する。
func (h assistConversationsHandler) open(w http.ResponseWriter, r *http.Request) {
	var body conversationOpenBody
	if apiError := decodeStrictBody(r, &body); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	if !body.Provider.IsKnown() {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: "provider must be a value the contract defines",
		})
		return
	}
	conversation, err := h.store.OpenConversation(body.Provider, body.Model)
	if err != nil {
		writeAssistStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, conversationOpenResponse{Conversation: conversation})
}

// turnBody は発言に添えた画面の文脈を登録する要求の本文である。
type turnBody struct {
	TurnId          string                      `json:"turnId"`
	MatchConditions []core.AssistMatchCondition `json:"matchConditions"`
	Case            string                      `json:"case,omitempty"`
	// SearchQuery は発言の時点で画面の検索欄が持っていた条件である。nodeKinds と granularity は、
	// 画面の図が出していた対象である。
	SearchQuery *core.SearchQuery `json:"searchQuery,omitempty"`
	// NodeKindsFromConditions は、画面が図に出す対象を検索の条件から決めていたときに真である。
	NodeKindsFromConditions bool `json:"nodeKindsFromConditions,omitempty"`
	// Records、NodeIds、EdgeIds は画面が選んでいた記録、ノード、関係である。本文は LLM が tool で読む。
	Records []core.RecordLocator `json:"records,omitempty"`
	NodeIds []string             `json:"nodeIds,omitempty"`
	EdgeIds []string             `json:"edgeIds,omitempty"`
}

// maxTurnSelections は発言 1 つが画面の選択として添えられる記録、ノード、関係の数の上限である。
//
// 既知の制限: 選択を種類ごとに 32 件までにする, 画面が選べるのは開いた記録 1 件と詳細を開いた
// ノードか関係 1 件であり、32 件は測る対象が無い, 画面が複数の選択を添える操作を持ったときに見直す
const maxTurnSelections = 32

// turnResponse は発言の登録の応答であり、LLM へ渡す本文である。
type turnResponse struct {
	TurnId                  string                      `json:"turnId"`
	MatchConditions         []core.AssistMatchCondition `json:"matchConditions"`
	Case                    string                      `json:"case,omitempty"`
	SearchQuery             *core.SearchQuery           `json:"searchQuery,omitempty"`
	NodeKindsFromConditions bool                        `json:"nodeKindsFromConditions,omitempty"`
	Records                 []assistRecordSummary       `json:"records"`
	Nodes                   []assistNodeSummary         `json:"nodes"`
	Edges                   []assistEdgeSummary         `json:"edges"`
}

// registerTurn は発言の文脈を登録し、画面の選択へ短い参照を発行する。
//
// **選択の本文を渡さない。** 渡すのは短い参照と、記録の収集元の file 名と位置、ノードと関係の
// 種類と表示名である。検索欄の条件は証拠の値を含むため、受け渡しとして記録してから渡す。
func (h assistConversationsHandler) registerTurn(w http.ResponseWriter, r *http.Request) {
	raw, apiError := readStrictBody(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	var body turnBody
	if apiError := decodeStrictBytes(raw, &body); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	turn := core.AssistTurn{
		ConversationId: r.PathValue(conversationIdPathValue), TurnId: body.TurnId,
		MatchConditions: body.MatchConditions, Case: body.Case,
	}
	if err := turn.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, *invalidRequestError(errors.New("the turn is not readable"), nil))
		return
	}
	// グラフを読む操作と同じく、関連付けの条件を 1 つ以上求める。
	if len(turn.MatchConditions) == 0 {
		writeError(w, http.StatusBadRequest, *invalidRequestError(
			errors.New("matchConditions must carry one condition or more"), nil))
		return
	}
	if err := selectionOfTurn(turn).Validate(); err != nil {
		writeError(w, http.StatusBadRequest, *invalidRequestError(err, nil))
		return
	}
	if len(body.Records) > maxTurnSelections || len(body.NodeIds) > maxTurnSelections ||
		len(body.EdgeIds) > maxTurnSelections {
		writeError(w, http.StatusBadRequest, *invalidRequestError(errors.New("the turn selects too many items"), nil))
		return
	}
	if body.SearchQuery != nil {
		if err := body.SearchQuery.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, *invalidRequestError(err, nil))
			return
		}
	}
	graph, versions, err := h.graphWithVersions(r.Context(), selectionOfTurn(turn), h.graphs)
	if err != nil {
		writeGraphFailure(w, r, err)
		return
	}
	h.disclose(w, pipeline.AssistDisclosureRequest{
		ConversationId: turn.ConversationId, TurnId: turn.TurnId, Tool: core.AssistToolTurn, Request: string(raw),
		Turn: &turn, Versions: versions,
	}, func(issuer pipeline.AssistRefIssuer) (pipeline.AssistComposition, error) {
		response := turnResponse{
			TurnId: turn.TurnId, MatchConditions: turn.MatchConditions, Case: turn.Case,
			SearchQuery: body.SearchQuery, NodeKindsFromConditions: body.NodeKindsFromConditions,
			Records: []assistRecordSummary{}, Nodes: []assistNodeSummary{}, Edges: []assistEdgeSummary{},
		}
		// 検索の条件の起点は、LLM へノードの識別子を渡さず短い参照で渡す。起点の種別と表示名は
		// ノードの要約に載せる。
		if body.SearchQuery != nil && len(body.SearchQuery.NodeIds) > 0 {
			query := *body.SearchQuery
			query.NodeIds = make([]string, 0, len(body.SearchQuery.NodeIds))
			for _, id := range body.SearchQuery.NodeIds {
				detail, found := graph.NodeDetail(id)
				if !found {
					return pipeline.AssistComposition{}, errAssistUnknownOrigin
				}
				summary := nodeSummaryOf(issuer, detail.Node)
				query.NodeIds = append(query.NodeIds, summary.Ref)
				response.Nodes = append(response.Nodes, summary)
			}
			response.SearchQuery = &query
		}
		var records []string
		for _, locator := range body.Records {
			summary, ok := h.recordSummary(issuer, locator)
			if !ok {
				return pipeline.AssistComposition{}, errAssistUnknownSelection
			}
			response.Records = append(response.Records, summary)
			records = append(records, summary.Ref)
		}
		for _, id := range body.NodeIds {
			detail, found := graph.NodeDetail(id)
			if !found {
				return pipeline.AssistComposition{}, errAssistUnknownSelection
			}
			response.Nodes = append(response.Nodes, nodeSummaryOf(issuer, detail.Node))
		}
		for _, id := range body.EdgeIds {
			detail, found := graph.EdgeDetail(id, pipeline.EdgeEvidenceFilter{})
			if !found {
				return pipeline.AssistComposition{}, errAssistUnknownSelection
			}
			response.Edges = append(response.Edges, edgeSummaryOf(issuer, detail.Edge))
		}
		return composeAssistBody(response, records, false)
	})
}

// errAssistUnknownSelection は、画面の選択が今の取り込み結果とグラフに無いことを表す。
var errAssistUnknownSelection = errors.New("a selected record, node or edge is not in the current graph")

// errAssistUnknownOrigin は、画面の検索の条件の起点のノードが今のグラフに無いことを表す。
var errAssistUnknownOrigin = errors.New("a node of the search conditions (nodeIds) is not in the current graph; " +
	"remove it from the search conditions")

// selectionOfTurn は発言の関連付けの条件を、グラフを選ぶ選択へ直す。
func selectionOfTurn(turn core.AssistTurn) pipeline.MatchConditionSelection {
	selection := pipeline.MatchConditionSelection{Conditions: []pipeline.SelectedMatchCondition{}}
	for _, condition := range turn.MatchConditions {
		selection.Conditions = append(selection.Conditions, pipeline.SelectedMatchCondition{
			ConditionKey: condition.ConditionKey, Tolerance: condition.Tolerance,
		})
	}
	return selection
}

// turnRequest は、登録した発言の中で行う受け渡しの要求が共通に持つ項目である。
type turnRequest struct {
	TurnId string `json:"turnId"`
}

// assistRead は、発言と、その発言の関連付けの条件のグラフと、グラフを読んだ時点の本文を組み直すのに
// 要る値である。
type assistRead struct {
	turn     core.AssistTurn
	graph    pipeline.Graph
	versions core.AssistVersions
}

// assistTurnOf は要求の発言を探し、その発言のグラフを返す。失敗したときは応答を書き、ok を偽に
// する。
func (h assistConversationsHandler) assistTurnOf(
	w http.ResponseWriter, r *http.Request, turnId string, graphs graphSource,
) (assistRead, bool) {
	turn, err := h.store.Turn(r.PathValue(conversationIdPathValue), turnId)
	if err != nil {
		writeAssistStoreError(w, err)
		return assistRead{}, false
	}
	graph, versions, err := h.graphWithVersions(r.Context(), selectionOfTurn(turn), graphs)
	if err != nil {
		writeGraphFailure(w, r, err)
		return assistRead{}, false
	}
	return assistRead{turn: turn, graph: graph, versions: versions}, true
}

// graphWithVersionsAttempts は、グラフと更新回数を同じ入力の状態で読む試みの回数の上限である。
const graphWithVersionsAttempts = 3

// errAnalystInputKeptChanging は、グラフを読む間に分析者の入力が変わり続けたことを表す。
var errAnalystInputKeptChanging = errors.New("the analyst input kept changing while the graph was read")

// graphWithVersions は、グラフと本文を組み直すのに要る値を、同じ入力の状態で読む。
//
// **読む前と後で分析者の入力の更新回数を比べる。** 間に端末の割当か時刻の解釈が記録されると、
// 本文の元のグラフと記録する更新回数がずれ、組み直しで照合できない受け渡しになる。ずれたときは
// 読み直す。
func (h assistConversationsHandler) graphWithVersions(
	ctx context.Context, selection pipeline.MatchConditionSelection, graphs graphSource,
) (pipeline.Graph, core.AssistVersions, error) {
	for range graphWithVersionsAttempts {
		before, err := h.versions()
		if err != nil {
			return pipeline.Graph{}, core.AssistVersions{}, err
		}
		graph, _, err := graphs(ctx, selection)
		if err != nil {
			return pipeline.Graph{}, core.AssistVersions{}, err
		}
		after, err := h.versions()
		if err != nil {
			return pipeline.Graph{}, core.AssistVersions{}, err
		}
		if before.AssignmentRevision == after.AssignmentRevision &&
			before.InterpretationRevision == after.InterpretationRevision {
			return graph, before, nil
		}
	}
	return pipeline.Graph{}, core.AssistVersions{}, errAnalystInputKeptChanging
}

// disclose は受け渡しの記録を書いてから、記録と同じ byte 列の本文を返す。request.Versions は、
// 本文の元のグラフを読んだ時点の値である。
func (h assistConversationsHandler) disclose(
	w http.ResponseWriter, request pipeline.AssistDisclosureRequest,
	compose func(pipeline.AssistRefIssuer) (pipeline.AssistComposition, error),
) {
	_, body, err := h.store.Disclose(request, compose)
	if err != nil {
		writeAssistStoreError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		// status line と header は送信済みである。本文が途中で切れた事象を記録するだけにする。
		slog.Error("writing the disclosure body failed", "error", err)
	}
}

// composeAssistBody は本文の組を、応答と同じ書き方の byte 列にする。
func composeAssistBody(value any, records []string, truncated bool) (pipeline.AssistComposition, error) {
	var buffer bytes.Buffer
	if err := output.WriteJSON(&buffer, value); err != nil {
		return pipeline.AssistComposition{}, err
	}
	return pipeline.AssistComposition{Body: buffer.Bytes(), RecordRefs: records, Truncated: truncated}, nil
}

// versions は本文を組み直すのに要る commit、改訂の番号、更新回数を読む。
func (h assistConversationsHandler) versions() (core.AssistVersions, error) {
	revision, modified := serverBuild()
	assignment, interpretation := pipeline.AnalystInputRevisions(h.investigation)
	sourceSet, err := sourceSetSha256(h.result)
	if err != nil {
		return core.AssistVersions{}, err
	}
	return core.AssistVersions{
		ServerRevision: revision, ServerModified: modified, AssignmentRevision: assignment,
		InterpretationRevision: interpretation, SourceSetSha256: sourceSet,
	}, nil
}

// serverBuild は server の build の VCS の revision と、commit に無い変更を含むかを返す。
var serverBuild = sync.OnceValues(func() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	var revision string
	var modified bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	return revision, modified
})

// sourceSetSha256 は、取り込んだ収集元の識別子と内容の識別と取得元の並びから sha256 を求める。
func sourceSetSha256(result pipeline.ImportResult) (string, error) {
	entries, err := result.SourceEntries()
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	for _, entry := range entries {
		for _, part := range []string{entry.Identity.SourceId, entry.Identity.ContentSha256, entry.Identity.OriginPath} {
			digest.Write([]byte(part))
			digest.Write([]byte{0})
		}
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// assistDisclosuresResponse は会話の受け渡しの記録の一覧である。**本型が項目の定義元である。**
type assistDisclosuresResponse struct {
	Disclosures []core.AssistDisclosure `json:"disclosures"`
}

// disclosures は会話の受け渡しの記録を記録した順に返す。画面が監査記録を閲覧する。
func (h assistConversationsHandler) disclosures(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if len(query) != 1 || len(query[conversationParam]) != 1 {
		writeError(w, http.StatusBadRequest, *invalidRequestError(
			errors.New("the operation takes the conversation once"), []string{conversationParam}))
		return
	}
	disclosures, err := h.store.Disclosures(query.Get(conversationParam))
	if err != nil {
		writeAssistStoreError(w, err)
		return
	}
	if disclosures == nil {
		disclosures = []core.AssistDisclosure{}
	}
	writeJSON(w, http.StatusOK, assistDisclosuresResponse{Disclosures: disclosures})
}

// writeAssistStoreError は保存先の失敗を、応答の code へ分けて返す。
//
// 退けた理由は LLM が次の手を判別できる文にする。
func writeAssistStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pipeline.ErrAssistUnavailable):
		writeError(w, http.StatusConflict, core.ApiError{
			Code: core.ApiErrorCodeAssistUnavailable, Message: "the launch keeps no investigation",
		})
	case errors.Is(err, pipeline.ErrAssistNotPermitted):
		writeError(w, http.StatusConflict, core.ApiError{
			Code:    core.ApiErrorCodeAssistNotPermitted,
			Message: "the investigation does not permit sending evidence to the provider; stop reading evidence",
		})
	case errors.Is(err, pipeline.ErrAssistConversationNotFound):
		writeError(w, http.StatusNotFound, core.ApiError{
			Code: core.ApiErrorCodeConversationNotFound, Message: "no conversation matches the identifier",
		})
	case errors.Is(err, pipeline.ErrAssistTurnNotFound):
		writeError(w, http.StatusBadRequest, *invalidRequestError(
			errors.New("the turn is not registered in the conversation"), nil))
	case errors.Is(err, pipeline.ErrAssistTurnExists):
		writeError(w, http.StatusBadRequest, *invalidRequestError(
			errors.New("the conversation already registered the turn"), nil))
	case errors.Is(err, errAssistUnknownOrigin):
		writeError(w, httpStatusFor(core.ApiErrorCodeRecordNotFound), core.ApiError{
			Code: core.ApiErrorCodeRecordNotFound, Message: err.Error(),
		})
	case errors.Is(err, errAssistUnknownSelection), errors.Is(err, errAssistUnknownRef):
		writeError(w, http.StatusBadRequest, *invalidRequestError(err, nil))
	default:
		writeStoreError(w, err, "recording the assist disclosure failed", "the assist request is not storable")
	}
}

// writeGraphFailure はグラフを取り出せなかった失敗を返す。要求の取り消しは応答を書かない。
func writeGraphFailure(w http.ResponseWriter, r *http.Request, err error) {
	if r.Context().Err() != nil {
		return
	}
	slog.Error("building the graph for the assist failed", "error", err)
	writeError(w, http.StatusInternalServerError, core.ApiError{
		Code: core.ApiErrorCodeInternalError, Message: "building the graph failed",
	})
}

// readStrictBody は要求の本文を上限まで読む。
func readStrictBody(r *http.Request) ([]byte, *core.ApiError) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, requestBodyLimit+1))
	if err != nil || len(raw) > requestBodyLimit {
		return nil, invalidRequestError(errors.New("the request body is not readable within the limit"), nil)
	}
	return raw, nil
}

// decodeStrictBytes は JSON の本文を、未知の項目と後続の値を退けて読む。
func decodeStrictBytes(raw []byte, into any) *core.ApiError {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return invalidRequestError(errors.New("the request body is not a readable JSON object of the operation"), nil)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return invalidRequestError(errors.New("the request body carries a value after its JSON object"), nil)
	}
	return nil
}

// decodeStrictBody は要求の本文を読み、decodeStrictBytes で読む。
func decodeStrictBody(r *http.Request, into any) *core.ApiError {
	raw, apiError := readStrictBody(r)
	if apiError != nil {
		return apiError
	}
	return decodeStrictBytes(raw, into)
}
