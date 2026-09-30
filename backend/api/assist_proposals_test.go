package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// assistProposalsPath は AI 提案の操作の path である。
const assistProposalsPath = "/api/v0/assist-proposals"

// proposalConversationId は会話の識別子である。
const proposalConversationId = "0123456789abcdef0123456789abcdef"

// assistProposalsResponse は AI 提案の一覧の応答の項目名を test 側で固定する。
type assistProposalsResponse struct {
	Proposals     []assistProposalItem `json:"proposals"`
	ProposalCount int64                `json:"proposalCount"`
	PendingCount  int64                `json:"pendingCount"`
	EmptyReason   core.EmptyReason     `json:"emptyReason,omitempty"`
}

type assistProposalItem struct {
	Proposal         core.AssistProposal `json:"proposal"`
	TargetOrigin     string              `json:"targetOrigin"`
	EndpointsInGraph *bool               `json:"endpointsInGraph,omitempty"`
}

type assistProposalAdoption struct {
	Proposal  assistProposalItem `json:"proposal"`
	Assertion assertionItem      `json:"assertion"`
}

type assistProposalDecided struct {
	core.ApiError
	Conflict assistProposalItem `json:"conflict"`
}

// proposalFixture は AI 提案を置ける保存先の handler と、その保存先と、fixture のグラフである。
type proposalFixture struct {
	handler   http.Handler
	proposals pipeline.AssistProposalStore
	graph     graphResponse
	// record は fixture のグラフの根拠のレコードの参照である。
	record core.AssertionRecordRef
}

func newProposalFixture(t *testing.T) proposalFixture {
	t.Helper()
	store, err := pipeline.NewMemoryStoreWithAssistProposals(graphImportResult(t), &testAssertionClock{})
	if err != nil {
		t.Fatal(err)
	}
	handler := handlerOf(store)
	encoded, err := json.Marshal(assertionRecordRefOf(t, handler))
	if err != nil {
		t.Fatal(err)
	}
	var record core.AssertionRecordRef
	if err := json.Unmarshal(encoded, &record); err != nil {
		t.Fatal(err)
	}
	return proposalFixture{handler: handler, proposals: store.AssistProposals(), graph: graphTargets(t, handler),
		record: record}
}

// propose は保存先へ提案を 1 件記録する。LLM の tool の作成を経由しない。
func (f proposalFixture) propose(t *testing.T, target core.AssertionTarget, addsRelation bool) core.AssistProposal {
	t.Helper()
	proposal, err := f.proposals.Create(pipeline.AssistProposalDraft{
		Target: target, Note: "LLM が挙げた候補", RecordRefs: []core.AssertionRecordRef{f.record},
		ConversationId: proposalConversationId, TurnId: "turn-1", Provider: core.AssistProviderClaude,
		Model: "model-a", AddsRelation: addsRelation,
		MatchConditions: []core.AssistMatchCondition{{ConditionKey: core.ConditionKeyDestinationIp}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return proposal
}

func (f proposalFixture) nodeTarget() core.AssertionTarget {
	return core.AssertionTarget{Kind: core.AssertionTargetKindNode, NodeId: f.graph.Nodes[0].Id}
}

func edgeTargetOf(edge graphEdge) core.AssertionTarget {
	return core.AssertionTarget{Kind: core.AssertionTargetKindEdge, Edge: &core.AssertionEdgeRef{
		Kind: core.EdgeKind(edge.Kind), SourceNodeId: edge.SourceNodeId, TargetNodeId: edge.TargetNodeId,
	}}
}

func requestProposal(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, withAllMatchConditions(path), strings.NewReader(body)))
	return response
}

func decodeProposals(t *testing.T, handler http.Handler) assistProposalsResponse {
	t.Helper()
	response := requestProposal(t, handler, http.MethodGet, assistProposalsPath, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var decoded assistProposalsResponse
	decodeJSON(t, response.Body, &decoded)
	return decoded
}

func adoptionPath(id string) string  { return assistProposalsPath + "/" + id + "/adoption" }
func rejectionPath(id string) string { return assistProposalsPath + "/" + id + "/rejection" }
func bodyOf(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func adopt(t *testing.T, handler http.Handler, id string, body any) assistProposalAdoption {
	t.Helper()
	response := requestProposal(t, handler, http.MethodPost, adoptionPath(id), bodyOf(t, body))
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d want %d body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	var decoded assistProposalAdoption
	decodeJSON(t, response.Body, &decoded)
	return decoded
}

// 調査の directory を渡さない起動は、AI 提案の一覧と採否を assist_unavailable で退ける。
func TestAssistProposalsAreUnavailableWithoutAnInvestigation(t *testing.T) {
	handler := graphHandler(t)
	for _, request := range []struct{ method, path, body string }{
		{http.MethodGet, assistProposalsPath, ""},
		{http.MethodPost, adoptionPath("ap:1"), `{"analyst":"analyst-a"}`},
		{http.MethodPost, rejectionPath("ap:1"), `{"analyst":"analyst-a"}`},
	} {
		response := requestProposal(t, handler, request.method, request.path, request.body)
		if response.Code != http.StatusConflict {
			t.Fatalf("%s %s status=%d want %d body=%s", request.method, request.path, response.Code,
				http.StatusConflict, response.Body.String())
		}
		var decoded core.ApiError
		decodeJSON(t, response.Body, &decoded)
		if decoded.Code != core.ApiErrorCodeAssistUnavailable {
			t.Errorf("%s %s code=%q want %q", request.method, request.path, decoded.Code,
				core.ApiErrorCodeAssistUnavailable)
		}
	}
}

// 提案は所見の一覧に入らず、AI 提案の一覧だけに提案中として出る。
func TestAssistProposalsAreListedApartFromTheAssertions(t *testing.T) {
	fixture := newProposalFixture(t)
	empty := decodeProposals(t, fixture.handler)
	if empty.ProposalCount != 0 || empty.EmptyReason != core.EmptyReasonNoRecordInFilter {
		t.Fatalf("empty listing = %+v, want no proposal with the empty reason", empty)
	}
	proposal := fixture.propose(t, fixture.nodeTarget(), false)
	listed := decodeProposals(t, fixture.handler)
	want := []assistProposalItem{{Proposal: proposal, TargetOrigin: string(core.AssertionTargetOriginObservation)}}
	if !reflect.DeepEqual(listed.Proposals, want) {
		t.Errorf("proposals = %+v, want %+v", listed.Proposals, want)
	}
	if listed.ProposalCount != int64(len(listed.Proposals)) || listed.PendingCount != 1 || listed.EmptyReason != "" {
		t.Errorf("counts = %d / %d, emptyReason=%q, want one pending proposal", listed.ProposalCount,
			listed.PendingCount, listed.EmptyReason)
	}
	if assertions := decodeAssertions(t, fixture.handler); assertions.AssertionCount != 0 {
		t.Errorf("assertions = %+v, want no assertion from a proposal", assertions.Assertions)
	}
}

// 採用は分析者を著者とする所見を作り、所見は分析者が直した記述と提案の根拠と元の提案の識別子を持つ。
func TestAdoptionCreatesTheAnalystAssertionFromTheProposal(t *testing.T) {
	fixture := newProposalFixture(t)
	proposal := fixture.propose(t, fixture.nodeTarget(), false)
	adopted := adopt(t, fixture.handler, proposal.Id, map[string]any{"analyst": "analyst-a", "note": "分析者が直した記述"})
	assertion := adopted.Assertion.Assertion
	if assertion.Author != "analyst-a" || assertion.Basis.Note != "分析者が直した記述" ||
		assertion.ProposalId != proposal.Id || !reflect.DeepEqual(assertion.Basis.RecordRefs, proposal.RecordRefs) ||
		!reflect.DeepEqual(assertion.Target, proposal.Target) || assertion.State != core.AssertionStateActive {
		t.Errorf("assertion = %+v, want the analyst assertion from the proposal", assertion)
	}
	decision := adopted.Proposal.Proposal.Decision
	if adopted.Proposal.Proposal.State != core.AssistProposalStateAdopted || decision == nil ||
		decision.AssertionId != assertion.Id || decision.Analyst != "analyst-a" ||
		decision.DecidedAt != assertion.RecordedAt {
		t.Errorf("proposal = %+v, want the adoption that names the assertion", adopted.Proposal.Proposal)
	}
	if adopted.Proposal.Proposal.Note != proposal.Note {
		t.Errorf("proposal note = %q, want the note the LLM wrote %q", adopted.Proposal.Proposal.Note, proposal.Note)
	}
	listed := decodeAssertions(t, fixture.handler)
	if len(listed.Assertions) != 1 || !reflect.DeepEqual(listed.Assertions[0].Assertion, assertion) {
		t.Errorf("assertions = %+v, want the adopted assertion", listed.Assertions)
	}
	if proposals := decodeProposals(t, fixture.handler); proposals.PendingCount != 0 {
		t.Errorf("pendingCount = %d, want 0 after the adoption", proposals.PendingCount)
	}
}

// 記述を省いた採用は、提案の記述を所見の記述にする。
func TestAdoptionWithoutANoteKeepsTheProposalNote(t *testing.T) {
	fixture := newProposalFixture(t)
	proposal := fixture.propose(t, fixture.nodeTarget(), false)
	adopted := adopt(t, fixture.handler, proposal.Id, map[string]any{"analyst": "analyst-a"})
	if adopted.Assertion.Assertion.Basis.Note != proposal.Note {
		t.Errorf("note = %q, want the proposal note %q", adopted.Assertion.Assertion.Basis.Note, proposal.Note)
	}
}

// 2 人が同じ提案を採用すると、2 件目は assist_proposal_decided で退けられ、所見は 1 件だけできる。
func TestSecondAdoptionOfTheSameProposalIsRefused(t *testing.T) {
	fixture := newProposalFixture(t)
	proposal := fixture.propose(t, fixture.nodeTarget(), false)
	statuses := make([]int, 2)
	var wait sync.WaitGroup
	for index, analyst := range []string{"analyst-a", "analyst-b"} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			response := httptest.NewRecorder()
			fixture.handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost,
				withAllMatchConditions(adoptionPath(proposal.Id)),
				strings.NewReader(`{"analyst":"`+analyst+`"}`)))
			statuses[index] = response.Code
		}()
	}
	wait.Wait()
	counts := map[int]int{}
	for _, status := range statuses {
		counts[status]++
	}
	if counts[http.StatusCreated] != 1 || counts[http.StatusConflict] != 1 {
		t.Fatalf("statuses = %v, want one adoption and one conflict", statuses)
	}
	if listed := decodeAssertions(t, fixture.handler); listed.AssertionCount != 1 {
		t.Fatalf("assertions = %+v, want the first adoption alone", listed.Assertions)
	}
	for _, path := range []string{adoptionPath(proposal.Id), rejectionPath(proposal.Id)} {
		response := requestProposal(t, fixture.handler, http.MethodPost, path, `{"analyst":"analyst-c"}`)
		if response.Code != http.StatusConflict {
			t.Fatalf("%s status=%d want %d body=%s", path, response.Code, http.StatusConflict, response.Body.String())
		}
		var decided assistProposalDecided
		decodeJSON(t, response.Body, &decided)
		if decided.Code != core.ApiErrorCodeAssistProposalDecided ||
			decided.Conflict.Proposal.State != core.AssistProposalStateAdopted {
			t.Errorf("%s answered %+v, want the adopted proposal as the conflict", path, decided)
		}
	}
}

// 却下は理由を任意で持ち、所見を作らない。
func TestRejectionKeepsTheReasonAndCreatesNoAssertion(t *testing.T) {
	fixture := newProposalFixture(t)
	proposal := fixture.propose(t, fixture.nodeTarget(), false)
	response := requestProposal(t, fixture.handler, http.MethodPost, rejectionPath(proposal.Id),
		`{"analyst":"analyst-b","reason":"根拠は別の端末のもの"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var rejected assistProposalItem
	decodeJSON(t, response.Body, &rejected)
	want := &core.AssistProposalDecision{Analyst: "analyst-b", DecidedAt: rejected.Proposal.Decision.DecidedAt,
		Reason: "根拠は別の端末のもの"}
	if rejected.Proposal.State != core.AssistProposalStateRejected || !reflect.DeepEqual(rejected.Proposal.Decision, want) {
		t.Errorf("proposal = %+v, want the rejection %+v", rejected.Proposal, want)
	}
	if listed := decodeAssertions(t, fixture.handler); listed.AssertionCount != 0 {
		t.Errorf("assertions = %+v, want none after a rejection", listed.Assertions)
	}
}

// listedProposal は一覧の中の、識別子 id の提案を返す。
func listedProposal(t *testing.T, handler http.Handler, id string) assistProposalItem {
	t.Helper()
	for _, item := range decodeProposals(t, handler).Proposals {
		if item.Proposal.Id == id {
			return item
		}
	}
	t.Fatalf("the listing lacks the proposal %q", id)
	return assistProposalItem{}
}

// 関係を足す提案の関係は、採用するまでグラフに無いため absent として出て、両端のノードがあることを
// 別に出す。採用で作る所見は関係を足し、所見の対象の出所は analyst_assertion になる。
func TestAdoptedRelationProposalAddsTheRelation(t *testing.T) {
	fixture := newProposalFixture(t)
	proposal := fixture.propose(t, edgeTargetOf(relationOutsideTheGraph(t, fixture.graph)), true)
	listed := listedProposal(t, fixture.handler, proposal.Id)
	if listed.TargetOrigin != string(core.AssertionTargetOriginAbsent) || listed.EndpointsInGraph == nil ||
		!*listed.EndpointsInGraph {
		t.Errorf("item = %+v, want an absent relation between present endpoints", listed)
	}
	adopted := adopt(t, fixture.handler, proposal.Id, map[string]any{"analyst": "analyst-a"})
	if !adopted.Assertion.Assertion.AddsRelation ||
		adopted.Assertion.TargetOrigin != string(core.AssertionTargetOriginAnalystAssertion) {
		t.Errorf("assertion = %+v, want an assertion that adds the relation", adopted.Assertion)
	}
}

// 関係を足す提案の関係が既にグラフにあれば、採用で作る所見は関係を足さず、出所はグラフの関係から決まる。
func TestAdoptedRelationProposalOnAnExistingRelationAddsNothing(t *testing.T) {
	fixture := newProposalFixture(t)
	observed := edgeOfState(t, fixture.graph, string(core.RelationStateObserved))
	proposal := fixture.propose(t, edgeTargetOf(observed), true)
	listed := listedProposal(t, fixture.handler, proposal.Id)
	if listed.TargetOrigin != string(core.AssertionTargetOriginObservation) {
		t.Errorf("targetOrigin = %q, want the observed relation", listed.TargetOrigin)
	}
	adopted := adopt(t, fixture.handler, proposal.Id, map[string]any{"analyst": "analyst-a"})
	if adopted.Assertion.Assertion.AddsRelation ||
		adopted.Assertion.TargetOrigin != string(core.AssertionTargetOriginObservation) {
		t.Errorf("assertion = %+v, want an assertion on the observed relation", adopted.Assertion)
	}
}

// 関係を足さない提案の関係がグラフに無ければ、採用で作る所見は関係を足さず、出所は absent になる。
func TestAdoptedProposalOnAVanishedRelationAddsNothing(t *testing.T) {
	fixture := newProposalFixture(t)
	proposal := fixture.propose(t, edgeTargetOf(relationOutsideTheGraph(t, fixture.graph)), false)
	listed := listedProposal(t, fixture.handler, proposal.Id)
	if listed.TargetOrigin != string(core.AssertionTargetOriginAbsent) || listed.EndpointsInGraph != nil {
		t.Errorf("item = %+v, want an absent relation without the endpoints item", listed)
	}
	adopted := adopt(t, fixture.handler, proposal.Id, map[string]any{"analyst": "analyst-a"})
	if adopted.Assertion.Assertion.AddsRelation ||
		adopted.Assertion.TargetOrigin != string(core.AssertionTargetOriginAbsent) {
		t.Errorf("assertion = %+v, want an assertion on an absent relation", adopted.Assertion)
	}
}

// 現在のグラフに無い対象の提案は absent として出る。関係を足す提案は、両端のノードが無いことも出す。
func TestProposalOnAVanishedTargetIsAbsent(t *testing.T) {
	fixture := newProposalFixture(t)
	vanishedNode := fixture.propose(t, core.AssertionTarget{Kind: core.AssertionTargetKindNode, NodeId: "n:absent"}, false)
	vanishedEnd := fixture.propose(t, core.AssertionTarget{Kind: core.AssertionTargetKindEdge,
		Edge: &core.AssertionEdgeRef{Kind: core.EdgeKindFileCopy, SourceNodeId: fixture.graph.Nodes[0].Id,
			TargetNodeId: "n:absent"}}, true)
	node := listedProposal(t, fixture.handler, vanishedNode.Id)
	if node.TargetOrigin != string(core.AssertionTargetOriginAbsent) || node.EndpointsInGraph != nil {
		t.Errorf("node item = %+v, want absent without the endpoints item", node)
	}
	end := listedProposal(t, fixture.handler, vanishedEnd.Id)
	if end.TargetOrigin != string(core.AssertionTargetOriginAbsent) || end.EndpointsInGraph == nil ||
		*end.EndpointsInGraph {
		t.Errorf("relation item = %+v, want absent with an endpoint missing", end)
	}
}

// failingAdoptionStore は採用を保存先の失敗で退ける AI 提案の保存先である。
type failingAdoptionStore struct {
	pipeline.AssistProposalStore
}

func (s failingAdoptionStore) Adopt(
	string, pipeline.AssistProposalAdoption,
) (core.AssistProposal, core.Assertion, error) {
	return core.AssistProposal{}, core.Assertion{}, fmt.Errorf("adopting: %w", pipeline.ErrAssertionStoreFailure)
}

// storeWithProposals は、AI 提案の保存先だけを差し替えた調査の保存先である。
type storeWithProposals struct {
	*pipeline.MemoryStore
	proposals pipeline.AssistProposalStore
}

func (s storeWithProposals) AssistProposals() pipeline.AssistProposalStore { return s.proposals }

// 保存先が採用を書けなかったときは、要求の不備と分けて 500 の internal_error で返す。
func TestAdoptionStoreFailureIsAnInternalError(t *testing.T) {
	memory, err := pipeline.NewMemoryStoreWithAssistProposals(graphImportResult(t), &testAssertionClock{})
	if err != nil {
		t.Fatal(err)
	}
	store := storeWithProposals{MemoryStore: memory, proposals: failingAdoptionStore{memory.AssistProposals()}}
	handler := handlerOf(store)
	nodeId := graphTargets(t, handler).Nodes[0].Id
	line := int64(1)
	proposal, err := store.proposals.Create(pipeline.AssistProposalDraft{
		Target: core.AssertionTarget{Kind: core.AssertionTargetKindNode, NodeId: nodeId}, Note: "候補",
		RecordRefs: []core.AssertionRecordRef{{SourceContentSha256: strings.Repeat("a", 64),
			PositionKind: core.PositionKindLineNumber, LineNumber: &line}},
		ConversationId: proposalConversationId, TurnId: "turn-1", Provider: core.AssistProviderClaude,
	})
	if err != nil {
		t.Fatal(err)
	}
	response := requestProposal(t, handler, http.MethodPost, adoptionPath(proposal.Id), `{"analyst":"analyst-a"}`)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want %d body=%s", response.Code, http.StatusInternalServerError, response.Body.String())
	}
	var decoded core.ApiError
	decodeJSON(t, response.Body, &decoded)
	if decoded.Code != core.ApiErrorCodeInternalError {
		t.Errorf("code=%q want %q", decoded.Code, core.ApiErrorCodeInternalError)
	}
}

// 採否の要求の不備と、無い提案を分けて退け、提案を提案中のまま残す。
func TestProposalDecisionRejectsTheMalformedRequest(t *testing.T) {
	fixture := newProposalFixture(t)
	proposal := fixture.propose(t, fixture.nodeTarget(), false)
	invalid, missing := core.ApiErrorCodeInvalidRequest, core.ApiErrorCodeRecordNotFound
	cases := []struct {
		name, path, body string
		want             int
		code             core.ApiErrorCode
	}{
		{"unknown item", adoptionPath(proposal.Id), `{"analyst":"analyst-a","state":"adopted"}`, http.StatusBadRequest, invalid},
		{"blank note", adoptionPath(proposal.Id), `{"analyst":"analyst-a","note":"  "}`, http.StatusBadRequest, invalid},
		{"no analyst", adoptionPath(proposal.Id), `{}`, http.StatusBadRequest, invalid},
		{"trailing value", rejectionPath(proposal.Id), `{"analyst":"analyst-a"} {}`, http.StatusBadRequest, invalid},
		{"no rejecting analyst", rejectionPath(proposal.Id), `{"reason":"理由"}`, http.StatusBadRequest, invalid},
		{"unknown proposal", adoptionPath("ap:missing"), `{"analyst":"analyst-a"}`, http.StatusNotFound, missing},
		{"unknown rejection", rejectionPath("ap:missing"), `{"analyst":"analyst-a"}`, http.StatusNotFound, missing},
	}
	for _, c := range cases {
		response := requestProposal(t, fixture.handler, http.MethodPost, c.path, c.body)
		if response.Code != c.want {
			t.Errorf("%s: status=%d want %d body=%s", c.name, response.Code, c.want, response.Body.String())
			continue
		}
		var decoded core.ApiError
		decodeJSON(t, response.Body, &decoded)
		if decoded.Code != c.code {
			t.Errorf("%s: code=%q want %q", c.name, decoded.Code, c.code)
		}
	}
	listed := decodeProposals(t, fixture.handler)
	if listed.PendingCount != 1 {
		t.Errorf("pendingCount = %d, want the proposal still proposed", listed.PendingCount)
	}
	if assertions := decodeAssertions(t, fixture.handler); assertions.AssertionCount != 0 {
		t.Errorf("assertions = %+v, want none after the refused requests", assertions.Assertions)
	}
}
