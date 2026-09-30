package api

import (
	"errors"
	"net/http"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// AI 提案の操作の pattern。採用と却下は提案の識別子を path に持つ。
//
// **画面が使う path であり、中継が退ける会話の path の外に置く。** 中継を起動していない分析者も
// 提案の採否を決められる。
const (
	assistProposalsPattern         = "GET /api/v0/assist-proposals"
	assistProposalAdoptionPattern  = "POST /api/v0/assist-proposals/{proposalId}/adoption"
	assistProposalRejectionPattern = "POST /api/v0/assist-proposals/{proposalId}/rejection"
	assistProposalIdPathValue      = "proposalId"
)

// assistProposalsHandler は AI 提案を一覧し、分析者の採用と却下を記録する。
//
// **提案は所見の一覧に入らない。** 採用だけが、分析者を著者とする所見を作る。
type assistProposalsHandler struct {
	proposals  pipeline.AssistProposalStore
	assertions pipeline.AssertionStore
	resolver   pipeline.AssertionResolver
}

// assistProposalsResponse は AI 提案の一覧の応答である。**本型が項目の定義元である。**
//
// 記録した提案を、採否を決めた提案も含めて全件返す。
type assistProposalsResponse struct {
	Proposals     []assistProposalItem `json:"proposals"`
	ProposalCount int64                `json:"proposalCount"`
	// PendingCount は状態が提案中の提案の件数である。
	PendingCount int64            `json:"pendingCount"`
	EmptyReason  core.EmptyReason `json:"emptyReason,omitempty"`
}

// assistProposalItem は AI 提案 1 件と、その対象が現在のグラフの何から出たかである。
type assistProposalItem struct {
	Proposal core.AssistProposal `json:"proposal"`
	// TargetOrigin は対象の出所である。値の意味は所見の対象の出所と同じであり、グラフが対象を持たない
	// 提案は absent になる。関係を足す提案の関係は、採用するまでグラフに無い。
	TargetOrigin core.AssertionTargetOrigin `json:"targetOrigin"`
	// EndpointsInGraph は、関係を足す提案の関係の両端のノードが現在のグラフにあることである。関係を足す
	// 提案だけが持つ。
	EndpointsInGraph *bool `json:"endpointsInGraph,omitempty"`
}

// assistProposalAdoptionResponse は採用の応答である。採用に変えた提案と、採用で作った所見を持つ。
type assistProposalAdoptionResponse struct {
	Proposal  assistProposalItem `json:"proposal"`
	Assertion assertionItem      `json:"assertion"`
}

// assistProposalAdoptionBody は採用の要求の本文である。
type assistProposalAdoptionBody struct {
	// Analyst は採用する分析者である。ログインした利用者の要求では省き、セッションのログイン名を使う。
	Analyst string `json:"analyst"`
	// Note は作る所見の記述である。省いた要求は提案の記述をそのまま使う。
	Note *string `json:"note,omitempty"`
}

// assistProposalRejectionBody は却下の要求の本文である。
type assistProposalRejectionBody struct {
	// Analyst は却下する分析者である。ログインした利用者の要求では省き、セッションのログイン名を使う。
	Analyst string `json:"analyst"`
	// Reason は却下の理由である。任意。
	Reason string `json:"reason,omitempty"`
}

// registerAssistProposals は AI 提案の操作を mux に登録する。
//
// **AI 提案を置けない起動は、グラフを組む前に assist_unavailable で退ける。** 対象の出所と関係を足すかを
// 読む操作は、要求の関連付けの条件のグラフを読む。
func registerAssistProposals(mux *http.ServeMux, store pipeline.InvestigationStore, graphs graphSource) {
	proposals := store.AssistProposals()
	route := func(serve func(assistProposalsHandler, http.ResponseWriter, *http.Request)) http.Handler {
		selecting := matchSelectingHandler{graphs: graphs, build: func(g pipeline.Graph, _ pipeline.ImportResult) http.Handler {
			handler := assistProposalsHandler{
				proposals: proposals, assertions: store.Assertions(), resolver: pipeline.NewAssertionResolver(g),
			}
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { serve(handler, w, r) })
		}}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !proposals.Available() {
				writeError(w, http.StatusConflict, core.ApiError{
					Code:    core.ApiErrorCodeAssistUnavailable,
					Message: "the launch keeps no investigation, so it records no assist proposal",
				})
				return
			}
			selecting.ServeHTTP(w, r)
		})
	}
	mux.Handle(assistProposalsPattern, route(assistProposalsHandler.list))
	mux.Handle(assistProposalAdoptionPattern, route(assistProposalsHandler.adopt))
	mux.Handle(assistProposalRejectionPattern, route(assistProposalsHandler.reject))
}

// list は記録した順の提案を全件返す。
func (h assistProposalsHandler) list(w http.ResponseWriter, r *http.Request) {
	if err := checkAssertionsParameterNames(r.URL.Query()); err != nil {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: err.Error(),
		})
		return
	}
	proposals := h.proposals.List()
	response := assistProposalsResponse{
		Proposals:     make([]assistProposalItem, 0, len(proposals)),
		ProposalCount: int64(len(proposals)),
	}
	for _, proposal := range proposals {
		response.Proposals = append(response.Proposals, h.item(proposal))
		if proposal.State == core.AssistProposalStateProposed {
			response.PendingCount++
		}
	}
	if response.ProposalCount == 0 {
		response.EmptyReason = core.EmptyReasonNoRecordInFilter
	}
	writeJSON(w, http.StatusOK, response)
}

// adopt は提案を採用し、分析者を著者とする所見を作る。
//
// **関係を足すかは採用する時点のグラフで決める。** 関係を足す提案の関係が、後の取り込みでグラフに
// 入っていれば、作る所見は関係を足さない。
func (h assistProposalsHandler) adopt(w http.ResponseWriter, r *http.Request) {
	var body assistProposalAdoptionBody
	if apiError := decodeStrictBody(r, &body); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	analyst, apiError := requestAuthor(r, body.Analyst)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	current, found := h.proposals.Find(r.PathValue(assistProposalIdPathValue))
	if !found {
		writeProposalNotFound(w)
		return
	}
	note := current.Note
	if body.Note != nil {
		note = *body.Note
	}
	proposal, assertion, err := h.proposals.Adopt(current.Id, pipeline.AssistProposalAdoption{
		Analyst: analyst, Note: note,
		AddsRelation: current.AddsRelation && !h.resolver.InGraph(current.Target),
	})
	if err != nil {
		h.writeDecisionError(w, proposal, err, "storing the adoption failed", "the adoption is not storable")
		return
	}
	writeJSON(w, http.StatusCreated, assistProposalAdoptionResponse{
		Proposal: h.item(proposal),
		Assertion: assertionItem{
			Assertion:    assertion,
			TargetOrigin: h.resolver.Origin(assertion, pipeline.NewAddedRelations(h.assertions.List())),
		},
	})
}

// reject は提案を却下する。
func (h assistProposalsHandler) reject(w http.ResponseWriter, r *http.Request) {
	var body assistProposalRejectionBody
	if apiError := decodeStrictBody(r, &body); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	analyst, apiError := requestAuthor(r, body.Analyst)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	proposal, err := h.proposals.Reject(r.PathValue(assistProposalIdPathValue), analyst, body.Reason)
	if err != nil {
		h.writeDecisionError(w, proposal, err, "storing the rejection failed", "the rejection is not storable")
		return
	}
	writeJSON(w, http.StatusOK, h.item(proposal))
}

// assistProposalDecidedResponse は assist_proposal_decided の応答である。Conflict は現在の提案を、
// 一覧の要素と同じ形で持つ。
type assistProposalDecidedResponse struct {
	core.ApiError
	Conflict assistProposalItem `json:"conflict"`
}

// writeDecisionError は採否の記録の失敗を、提案が無い・採否を決めた後・要求の不備・内部の失敗に分けて返す。
func (h assistProposalsHandler) writeDecisionError(
	w http.ResponseWriter, current core.AssistProposal, err error, internalMessage, requestMessage string,
) {
	switch {
	case errors.Is(err, pipeline.ErrAssistProposalNotFound):
		writeProposalNotFound(w)
	case errors.Is(err, pipeline.ErrAssistProposalDecided):
		writeJSON(w, http.StatusConflict, assistProposalDecidedResponse{
			ApiError: core.ApiError{
				Code: core.ApiErrorCodeAssistProposalDecided, Message: "the proposal is already decided",
			},
			Conflict: h.item(current),
		})
	default:
		writeStoreError(w, err, internalMessage, requestMessage)
	}
}

func writeProposalNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, core.ApiError{
		Code: core.ApiErrorCodeRecordNotFound, Message: "no assist proposal matches the identifier",
	})
}

func (h assistProposalsHandler) item(proposal core.AssistProposal) assistProposalItem {
	item := assistProposalItem{Proposal: proposal, TargetOrigin: h.resolver.ProposalOrigin(proposal)}
	if proposal.AddsRelation && proposal.Target.Edge != nil {
		present := h.resolver.EndpointsInGraph(*proposal.Target.Edge)
		item.EndpointsInGraph = &present
	}
	return item
}
