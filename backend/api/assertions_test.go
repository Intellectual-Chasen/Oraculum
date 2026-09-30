package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// assertionsPath は分析者の所見の操作の path である。
const assertionsPath = "/api/v0/assertions"

// assertionsResponse は所見の一覧の応答の項目名を test 側で固定する。decodeJSON が
// DisallowUnknownFields で読むため、handler が項目を足すと test が失敗する。
type assertionsResponse struct {
	Assertions     []assertionItem  `json:"assertions"`
	AssertionCount int64            `json:"assertionCount"`
	EmptyReason    core.EmptyReason `json:"emptyReason,omitempty"`
}

type assertionItem struct {
	Assertion    core.Assertion `json:"assertion"`
	TargetOrigin string         `json:"targetOrigin"`
}

// graphTargets は fixture のグラフから、所見の対象に使うノードとエッジを返す。
func graphTargets(t *testing.T, handler http.Handler) graphResponse {
	t.Helper()
	decoded := decodeGraph(t, handler, wholeGraphQuery)
	if len(decoded.Nodes) == 0 || len(decoded.Edges) == 0 {
		t.Fatalf("the fixture graph carries %d nodes and %d edges, want both populated",
			len(decoded.Nodes), len(decoded.Edges))
	}
	return decoded
}

// edgeOfState は求める状態のエッジ 1 本を返す。
func edgeOfState(t *testing.T, graph graphResponse, state string) graphEdge {
	t.Helper()
	for _, edge := range graph.Edges {
		if edge.State == state {
			return edge
		}
	}
	t.Fatalf("the fixture graph carries no edge in the state %q", state)
	return graphEdge{}
}

// edgeTargetBody は関係を対象にした要求の本文を組む。
func edgeTargetBody(edge graphEdge, note string) string {
	body := map[string]any{
		"target": map[string]any{
			"kind": "edge",
			"edge": map[string]any{
				"kind": edge.Kind, "sourceNodeId": edge.SourceNodeId,
				"targetNodeId": edge.TargetNodeId,
			},
		},
		"author": "analyst-a",
		"basis":  map[string]any{"note": note},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// nodeTargetBody はノードを対象にした要求の本文を組む。
func nodeTargetBody(nodeId, note string) string {
	encoded, err := json.Marshal(map[string]any{
		"target": map[string]any{"kind": "node", "nodeId": nodeId},
		"author": "analyst-a",
		"basis":  map[string]any{"note": note},
	})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func requestAssertion(
	t *testing.T, handler http.Handler, method, path, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, withAllMatchConditions(path), strings.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeAssertionItem(
	t *testing.T, response *httptest.ResponseRecorder, wantStatus int,
) assertionItem {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status=%d want %d body=%s", response.Code, wantStatus, response.Body.String())
	}
	var decoded assertionItem
	decodeJSON(t, response.Body, &decoded)
	return decoded
}

// decodeAssertions は所見の一覧を読む。一覧は要求の項目を取らないため、path だけを送る。
func decodeAssertions(t *testing.T, handler http.Handler) assertionsResponse {
	t.Helper()
	response := requestAssertion(t, handler, http.MethodGet, assertionsPath, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var decoded assertionsResponse
	decodeJSON(t, response.Body, &decoded)
	return decoded
}

// 記録した所見は、著者・時刻・対象・根拠・変更履歴を持って返る。
func TestAssertionsEndpointRecordsTheAnalystJudgement(t *testing.T) {
	handler := graphHandler(t)
	graph := graphTargets(t, handler)
	observed := edgeOfState(t, graph, string(core.RelationStateObserved))

	created := decodeAssertionItem(t, requestAssertion(t, handler, http.MethodPost, assertionsPath,
		edgeTargetBody(observed, "同じ秒の別の候補を採る")),
		http.StatusCreated)

	if created.Assertion.Author != "analyst-a" {
		t.Errorf("author = %q, want the author of the request", created.Assertion.Author)
	}
	if created.Assertion.Target.Edge == nil ||
		created.Assertion.Target.Edge.SourceNodeId != observed.SourceNodeId {
		t.Errorf("target = %+v, want the edge of the request", created.Assertion.Target)
	}
	if err := created.Assertion.RecordedAt.Validate(); err != nil {
		t.Errorf("recordedAt = %q: %v", created.Assertion.RecordedAt, err)
	}
	if created.Assertion.Basis.Note == "" {
		t.Error("the response carries no basis note, want the note of the request")
	}
	if created.Assertion.RevisionNumber != core.FirstAssertionRevisionNumber {
		t.Errorf("revisionNumber = %d, want %d",
			created.Assertion.RevisionNumber, core.FirstAssertionRevisionNumber)
	}
	if len(created.Assertion.History) != 0 {
		t.Errorf("history = %+v, want the first revision to supersede nothing",
			created.Assertion.History)
	}
}

// 応答は観測から直に出した関係、関連付けが挙げた候補、分析者が足した関係、取り込みに無い
// 対象を分ける。
func TestAssertionsEndpointSeparatesTheOriginOfTheTarget(t *testing.T) {
	handler := graphHandler(t)
	graph := graphTargets(t, handler)
	observed := edgeOfState(t, graph, string(core.RelationStateObserved))
	candidate := edgeOfState(t, graph, string(core.RelationStateCandidate))
	added := relationOutsideTheGraph(t, graph)

	cases := []struct {
		name string
		body string
		want core.AssertionTargetOrigin
	}{
		{
			name: "observed edge",
			body: edgeTargetBody(observed, "観測のエッジを否認する"),
			want: core.AssertionTargetOriginObservation,
		},
		{
			name: "matching candidate edge",
			body: edgeTargetBody(candidate, "候補のエッジを否認する"),
			want: core.AssertionTargetOriginMatchingCandidate,
		},
		{
			name: "edge the analyst added",
			body: edgeTargetBody(added, "同じ内容の複製を足す"),
			want: core.AssertionTargetOriginAnalystAssertion,
		},
		{
			name: "node outside the current import",
			body: nodeTargetBody("n:process:absent",
				"別の取り込みで付けた注釈"),
			want: core.AssertionTargetOriginAbsent,
		},
		{
			name: "node of the current import",
			body: nodeTargetBody(graph.Nodes[0].Id,
				"観測したノードへの注釈"),
			want: core.AssertionTargetOriginObservation,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			item := decodeAssertionItem(t,
				requestAssertion(t, handler, http.MethodPost, assertionsPath, testCase.body),
				http.StatusCreated)
			if item.TargetOrigin != string(testCase.want) {
				t.Errorf("targetOrigin = %q, want %q", item.TargetOrigin, testCase.want)
			}
		})
	}
}

// 所見を付けた関係は、観測層のグラフに同じ状態で残り、所見は別に残る。
func TestAssertionKeepsTheEdgeInTheGraph(t *testing.T) {
	handler := graphHandler(t)
	before := graphTargets(t, handler)
	observed := edgeOfState(t, before, string(core.RelationStateObserved))

	created := decodeAssertionItem(t, requestAssertion(t, handler, http.MethodPost, assertionsPath,
		edgeTargetBody(observed, "同じ秒の別の候補を採る")),
		http.StatusCreated)

	after := decodeGraph(t, handler, wholeGraphQuery)
	if len(after.Edges) != len(before.Edges) || len(after.Nodes) != len(before.Nodes) {
		t.Fatalf("the graph carries %d nodes and %d edges after the assertion, want %d and %d",
			len(after.Nodes), len(after.Edges), len(before.Nodes), len(before.Edges))
	}
	for index, edge := range after.Edges {
		if edge.Id != before.Edges[index].Id || edge.State != before.Edges[index].State {
			t.Errorf("edge %d became %q %q, want %q %q", index, edge.Id, edge.State,
				before.Edges[index].Id, before.Edges[index].State)
		}
	}
	listed := decodeAssertions(t, handler)
	if len(listed.Assertions) != 1 || listed.Assertions[0].Assertion.Id != created.Assertion.Id {
		t.Fatalf("the listing carries %+v, want the assertion %q", listed.Assertions,
			created.Assertion.Id)
	}
	recorded := listed.Assertions[0]
	if recorded.Assertion.State != core.AssertionStateActive {
		t.Errorf("the assertion is %q, want %q",
			recorded.Assertion.State, core.AssertionStateActive)
	}
	if recorded.TargetOrigin != string(core.AssertionTargetOriginObservation) {
		t.Errorf("targetOrigin = %q, want the edge of the assertion to stay an observation",
			recorded.TargetOrigin)
	}
}

// グラフが持たない関係へ付けた所見は、分析者が足した関係として出所を返す。
func TestAssertionsOnTheAddedRelationCarryTheAnalystOrigin(t *testing.T) {
	handler := graphHandler(t)
	graph := graphTargets(t, handler)
	added := relationOutsideTheGraph(t, graph)
	addition := decodeAssertionItem(t, requestAssertion(t, handler, http.MethodPost, assertionsPath,
		edgeTargetBody(added, "同じ内容の複製を足す")),
		http.StatusCreated)

	following := decodeAssertionItem(t,
		requestAssertion(t, handler, http.MethodPost, assertionsPath,
			edgeTargetBody(added, "足した関係への所見")), http.StatusCreated)
	if following.TargetOrigin != string(core.AssertionTargetOriginAnalystAssertion) {
		t.Errorf("targetOrigin = %q, want %q", following.TargetOrigin,
			core.AssertionTargetOriginAnalystAssertion)
	}

	// 関係を主張する所見をすべて取り下げると、出所が absent になる。
	withdrawal, err := json.Marshal(map[string]any{
		"target": addition.Assertion.Target,
		"author": "analyst-b", "state": string(core.AssertionStateWithdrawn),
		"basis":        map[string]any{"note": "複製の根拠を取り下げる"},
		"baseRevision": core.FirstAssertionRevisionNumber,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{addition.Assertion.Id, following.Assertion.Id} {
		decodeAssertionItem(t, requestAssertion(t, handler, http.MethodPut,
			assertionsPath+"/"+id, string(withdrawal)), http.StatusOK)
	}

	listed := decodeAssertions(t, handler)
	for _, item := range listed.Assertions {
		if item.TargetOrigin != string(core.AssertionTargetOriginAbsent) {
			t.Errorf("targetOrigin of %q = %q, want %q after every assertion on the relation "+
				"is withdrawn", item.Assertion.Id, item.TargetOrigin,
				core.AssertionTargetOriginAbsent)
		}
	}
}

// 所見はノード・関係・レコードのいずれも対象にでき、除去した固定フォーマットの項目を
// 持つ要求を退ける。
func TestAssertionsEndpointTakesEveryTargetWithoutTheFixedFormat(t *testing.T) {
	handler := graphHandler(t)
	graph := graphTargets(t, handler)

	onANode := decodeAssertionItem(t, requestAssertion(t, handler, http.MethodPost, assertionsPath,
		nodeTargetBody(graph.Nodes[0].Id, "ノードへのメモ")), http.StatusCreated)
	if onANode.TargetOrigin != string(core.AssertionTargetOriginObservation) {
		t.Errorf("targetOrigin = %q, want %q for a node of the import",
			onANode.TargetOrigin, core.AssertionTargetOriginObservation)
	}
	onARelation := decodeAssertionItem(t, requestAssertion(t, handler, http.MethodPost,
		assertionsPath, edgeTargetBody(relationOutsideTheGraph(t, graph), "関係へのメモ")),
		http.StatusCreated)
	// 出所が analyst_assertion であることが、対象をグラフが持たないことを表す。
	if onARelation.TargetOrigin != string(core.AssertionTargetOriginAnalystAssertion) {
		t.Errorf("targetOrigin = %q, want %q for a relation the graph lacks",
			onARelation.TargetOrigin, core.AssertionTargetOriginAnalystAssertion)
	}

	// 反対側。除去した項目を持つ要求は、未知の項目として退く。
	for _, item := range []map[string]any{
		{"kind": "annotation"},
		{"technique": map[string]any{
			"catalogName": "catalog", "catalogVersion": "v1",
			"techniqueId": "T1136.001", "sourceUrl": "https://example.test/T1136/001/",
		}},
	} {
		body := map[string]any{
			"target": map[string]any{"kind": "node", "nodeId": graph.Nodes[0].Id},
			"author": "analyst-a",
			"basis":  map[string]any{"note": "メモ"},
		}
		for name, value := range item {
			body[name] = value
		}
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		response := requestAssertion(t, handler, http.MethodPost, assertionsPath, string(encoded))
		if response.Code != http.StatusBadRequest {
			t.Errorf("status=%d want %d for a request carrying %+v body=%s",
				response.Code, http.StatusBadRequest, item, response.Body.String())
		}
	}
}

// relationOutsideTheGraph は、両端が観測層にありながらグラフが持たない関係を返す。
func relationOutsideTheGraph(t *testing.T, graph graphResponse) graphEdge {
	t.Helper()
	observed := edgeOfState(t, graph, string(core.RelationStateObserved))
	candidate := edgeOfState(t, graph, string(core.RelationStateCandidate))
	added := graphEdge{
		Kind: string(core.EdgeKindFileCopy), SourceNodeId: observed.SourceNodeId,
		TargetNodeId: candidate.TargetNodeId,
	}
	for _, edge := range graph.Edges {
		if edge.Kind == added.Kind && edge.SourceNodeId == added.SourceNodeId &&
			edge.TargetNodeId == added.TargetNodeId {
			t.Fatalf("the fixture graph carries the relation %+v, want a relation it lacks", added)
		}
	}
	return added
}

// 一覧の全件から、同じ関係を指す所見の出所を決める。
func TestAssertionsListingReadsTheAddedRelationsFromTheWholeListing(t *testing.T) {
	handler := graphHandler(t)
	graph := graphTargets(t, handler)
	added := relationOutsideTheGraph(t, graph)
	addition := decodeAssertionItem(t, requestAssertion(t, handler, http.MethodPost, assertionsPath,
		edgeTargetBody(added, "同じ内容の複製を足す")),
		http.StatusCreated)
	annotation := decodeAssertionItem(t, requestAssertion(t, handler, http.MethodPost,
		assertionsPath, edgeTargetBody(added, "足した関係への注釈")),
		http.StatusCreated)

	listed := decodeAssertions(t, handler)
	for _, want := range []string{addition.Assertion.Id, annotation.Assertion.Id} {
		item := assertionItemOfId(t, listed, want)
		if item.TargetOrigin != string(core.AssertionTargetOriginAnalystAssertion) {
			t.Errorf("targetOrigin of %q = %q, want %q", want, item.TargetOrigin,
				core.AssertionTargetOriginAnalystAssertion)
		}
	}
}

// assertionItemOfId は一覧から識別子で指定した所見 1 件を返す。
func assertionItemOfId(t *testing.T, listed assertionsResponse, id string) assertionItem {
	t.Helper()
	for _, item := range listed.Assertions {
		if item.Assertion.Id == id {
			return item
		}
	}
	t.Fatalf("the listing carries %+v, want the assertion %q", listed.Assertions, id)
	return assertionItem{}
}

// 保存先の内部の失敗を internal_error で返し、要求の不備と分ける。
func TestAssertionsEndpointSeparatesTheStoreFailureFromTheRequestFault(t *testing.T) {
	failing := &failingAssertionStore{}
	handler := handlerOf(investigationStoreOf(graphImportResult(t), failing))
	body := nodeTargetBody("n:process:a", "根拠")

	response := requestAssertion(t, handler, http.MethodPost, assertionsPath, body)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want %d body=%s", response.Code, http.StatusInternalServerError,
			response.Body.String())
	}
	var apiError core.ApiError
	decodeJSON(t, response.Body, &apiError)
	if apiError.Code != core.ApiErrorCodeInternalError {
		t.Errorf("code = %q, want %q", apiError.Code, core.ApiErrorCodeInternalError)
	}
	if len(failing.drafts) != 1 {
		t.Fatalf("the store received %+v, want the one draft of the request", failing.drafts)
	}
	draft := failing.drafts[0]
	if draft.Author != "analyst-a" ||
		draft.Basis.Note != "根拠" || draft.Target.NodeId != "n:process:a" {
		t.Errorf("the store received %+v, want the author, the basis and the target "+
			"of the request", draft)
	}

	// 要求の不備は同じ保存先でも 400 になる。保存先へ届く前に退けるためである。
	faulty := requestAssertion(t, handler, http.MethodPost, assertionsPath,
		`{"target":{"kind":"suspicion","nodeId":"n:process:a"},"author":"analyst-a",`+
			`"basis":{"note":"根拠"}}`)
	if faulty.Code != http.StatusBadRequest {
		t.Errorf("status=%d want %d for a request fault body=%s",
			faulty.Code, http.StatusBadRequest, faulty.Body.String())
	}
	if len(failing.drafts) != 1 {
		t.Errorf("the store received %+v, want the faulty request to stop before the store",
			failing.drafts)
	}
}

// failingAssertionStore は保存先の失敗を返し、受け取った入力を保つ。
type failingAssertionStore struct {
	drafts []pipeline.AssertionDraft
	// existing は Find が返す所見である。識別子が空の組は、該当する所見が無い状態を表す。
	existing core.Assertion
	// reviseError は Revise が返す失敗である。
	reviseError error
	// revisions は Revise が受け取った入力である。
	revisions []assertionRevisionCall
}

// assertionRevisionCall は Revise 1 回分の入力である。
type assertionRevisionCall struct {
	assertionId string
	draft       pipeline.AssertionRevisionDraft
}

func (s *failingAssertionStore) List() []core.Assertion { return nil }

func (s *failingAssertionStore) Find(assertionId string) (core.Assertion, bool) {
	if s.existing.Id != assertionId {
		return core.Assertion{}, false
	}
	return s.existing, true
}

func (s *failingAssertionStore) Create(
	draft pipeline.AssertionDraft,
) (core.Assertion, error) {
	s.drafts = append(s.drafts, draft)
	return core.Assertion{}, fmt.Errorf("creating the assertion: %w",
		pipeline.ErrAssertionStoreFailure)
}

func (s *failingAssertionStore) Revise(
	assertionId string, draft pipeline.AssertionRevisionDraft,
) (core.Assertion, error) {
	s.revisions = append(s.revisions,
		assertionRevisionCall{assertionId: assertionId, draft: draft})
	return core.Assertion{}, s.reviseError
}

// 改訂の追加は、該当する所見が無い失敗を record_not_found で返し、保存先の内部の失敗を
// internal_error で返す。
func TestAssertionRevisionSeparatesTheStoreFailureFromTheMissingAssertion(t *testing.T) {
	existing := core.Assertion{
		Id:             "as:0001",
		Target:         core.AssertionTarget{Kind: core.AssertionTargetKindNode, NodeId: "n:process:a"},
		State:          core.AssertionStateActive,
		Author:         "analyst-a",
		RecordedAt:     "2026-01-02T03:04:05.000Z",
		Basis:          core.AssertionBasis{Note: "最初の根拠"},
		RevisionNumber: core.FirstAssertionRevisionNumber,
	}
	encoded, err := json.Marshal(map[string]any{
		"target": existing.Target,
		"author": "analyst-b", "state": string(core.AssertionStateWithdrawn),
		"basis":        map[string]any{"note": "2 つ目の根拠"},
		"baseRevision": existing.RevisionNumber,
	})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name        string
		reviseError error
		wantStatus  int
		wantCode    core.ApiErrorCode
	}{
		{
			name:        "the identifier matches no assertion",
			reviseError: pipeline.ErrAssertionNotFound,
			wantStatus:  http.StatusNotFound,
			wantCode:    core.ApiErrorCodeRecordNotFound,
		},
		{
			name: "the store cannot hold the revision",
			reviseError: fmt.Errorf("revising the assertion: %w",
				pipeline.ErrAssertionStoreFailure),
			wantStatus: http.StatusInternalServerError,
			wantCode:   core.ApiErrorCodeInternalError,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			failing := &failingAssertionStore{existing: existing, reviseError: testCase.reviseError}
			handler := handlerOf(investigationStoreOf(graphImportResult(t), failing))

			response := requestAssertion(t, handler, http.MethodPut,
				assertionsPath+"/"+existing.Id, string(encoded))
			if response.Code != testCase.wantStatus {
				t.Fatalf("status=%d want %d body=%s", response.Code, testCase.wantStatus,
					response.Body.String())
			}
			var apiError core.ApiError
			decodeJSON(t, response.Body, &apiError)
			if apiError.Code != testCase.wantCode {
				t.Errorf("code = %q, want %q", apiError.Code, testCase.wantCode)
			}
			if len(failing.revisions) != 1 {
				t.Fatalf("the store received %+v, want the one revision of the request",
					failing.revisions)
			}
			call := failing.revisions[0]
			if call.assertionId != existing.Id || call.draft.State != core.AssertionStateWithdrawn ||
				call.draft.Author != "analyst-b" || call.draft.Basis.Note != "2 つ目の根拠" {
				t.Errorf("the store received %+v, want the identifier, the state, the author and "+
					"the basis of the request", call)
			}
		})
	}
}

// investigationStore は取り込み結果と、検査が渡した所見と AI 支援の記録の保存先を組にする。
type investigationStore struct {
	result      pipeline.ImportResult
	assertions  pipeline.AssertionStore
	assignments pipeline.TerminalAssignmentStore
	assist      pipeline.AssistStore
}

func investigationStoreOf(
	result pipeline.ImportResult, assertions pipeline.AssertionStore,
) pipeline.InvestigationStore {
	return investigationStore{
		result: result, assertions: assertions,
		assignments: pipeline.NewMemoryTerminalAssignmentStore(),
		assist:      pipeline.UnavailableAssistStore(),
	}
}

func (s investigationStore) Assist() pipeline.AssistStore { return s.assist }

func (s investigationStore) ImportResult() pipeline.ImportResult { return s.result }

func (s investigationStore) Assertions() pipeline.AssertionStore { return s.assertions }

func (s investigationStore) AssistProposals() pipeline.AssistProposalStore {
	return pipeline.NewUnavailableAssistProposalStore()
}

func (s investigationStore) TerminalAssignments() pipeline.TerminalAssignmentStore {
	return s.assignments
}

// 改訂を足すと、置き換えられた改訂が履歴に残り、取り下げた所見も残る。
func TestAssertionRevisionKeepsTheSupersededRevision(t *testing.T) {
	handler := graphHandler(t)
	graph := graphTargets(t, handler)
	observed := edgeOfState(t, graph, string(core.RelationStateObserved))
	created := decodeAssertionItem(t, requestAssertion(t, handler, http.MethodPost, assertionsPath,
		edgeTargetBody(observed, "最初の根拠")), http.StatusCreated)

	body := map[string]any{
		"target":       created.Assertion.Target,
		"author":       "analyst-b",
		"state":        string(core.AssertionStateWithdrawn),
		"basis":        map[string]any{"note": "別のレコードで否定された"},
		"baseRevision": created.Assertion.RevisionNumber,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	revised := decodeAssertionItem(t, requestAssertion(t, handler, http.MethodPut,
		assertionsPath+"/"+created.Assertion.Id, string(encoded)), http.StatusOK)

	if revised.Assertion.State != core.AssertionStateWithdrawn {
		t.Errorf("state = %q, want %q", revised.Assertion.State, core.AssertionStateWithdrawn)
	}
	if revised.Assertion.RevisionNumber != created.Assertion.RevisionNumber+1 {
		t.Errorf("revisionNumber = %d, want %d", revised.Assertion.RevisionNumber,
			created.Assertion.RevisionNumber+1)
	}
	if len(revised.Assertion.History) != 1 {
		t.Fatalf("history = %+v, want the one superseded revision", revised.Assertion.History)
	}
	superseded := revised.Assertion.History[0]
	if superseded.Author != created.Assertion.Author ||
		superseded.Basis.Note != created.Assertion.Basis.Note ||
		superseded.RecordedAt != created.Assertion.RecordedAt {
		t.Errorf("the superseded revision %+v differs from the created assertion %+v",
			superseded, created.Assertion)
	}
	listed := decodeAssertions(t, handler)
	if len(listed.Assertions) != 1 ||
		listed.Assertions[0].Assertion.State != core.AssertionStateWithdrawn {
		t.Errorf("the listing carries %+v, want the withdrawn assertion to stay", listed.Assertions)
	}
}

// 改訂の追加が対象を変える要求を退け、同じ対象の要求を通す。
func TestAssertionRevisionKeepsTheTarget(t *testing.T) {
	handler := graphHandler(t)
	graph := graphTargets(t, handler)
	observed := edgeOfState(t, graph, string(core.RelationStateObserved))
	created := decodeAssertionItem(t, requestAssertion(t, handler, http.MethodPost, assertionsPath,
		edgeTargetBody(observed, "最初の根拠")), http.StatusCreated)

	revision := func(target any) string {
		encoded, err := json.Marshal(map[string]any{
			"target": target, "author": "analyst-b",
			"state":        string(core.AssertionStateActive),
			"basis":        map[string]any{"note": "2 つ目の根拠"},
			"baseRevision": created.Assertion.RevisionNumber,
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	path := assertionsPath + "/" + created.Assertion.Id

	changed := requestAssertion(t, handler, http.MethodPut, path,
		revision(map[string]any{"kind": "node", "nodeId": graph.Nodes[0].Id}))
	if changed.Code != http.StatusBadRequest {
		t.Errorf("status=%d want %d for a revision changing the target",
			changed.Code, http.StatusBadRequest)
	}
	kept := requestAssertion(t, handler, http.MethodPut, path,
		revision(created.Assertion.Target))
	if kept.Code != http.StatusOK {
		t.Errorf("status=%d want %d for a revision keeping the target body=%s",
			kept.Code, http.StatusOK, kept.Body.String())
	}
}

// noteOnRecordBody は原資料のレコードを対象にしたメモの要求の本文を組む。
func noteOnRecordBody(t *testing.T, record map[string]any, note string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"target": map[string]any{"kind": "record", "record": record},
		"author": "analyst-a",
		"basis": map[string]any{
			"note":       note,
			"recordRefs": []any{record},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// assertionRecordRefOf は fixture のグラフの根拠から、所見が指すレコードの参照を組む。
func assertionRecordRefOf(t *testing.T, handler http.Handler) map[string]any {
	t.Helper()
	for _, edge := range decodeGraph(t, handler, wholeGraphQuery).Edges {
		// **`/api/v0/graph` は根拠の中身を持たない。** 根拠は `/api/v0/edges/{id}` から読む。
		evidence := decodeEdge(t, handler, edge.Id, "").Edge.Evidence
		if len(evidence) == 0 {
			continue
		}
		locator := evidence[0].RecordRef
		// **位置の欄は、持っているものをすべて写す。** 所見の参照は core が組む鍵と
		// 同じ材料で探すため、片方だけを写すと別のレコードの鍵になる。
		ref := map[string]any{
			"sourceContentSha256": locator.SourceContentSha256,
			"positionKind":        string(locator.PositionKind),
		}
		if locator.SequenceNumber != nil {
			ref["sequenceNumber"] = *locator.SequenceNumber
		}
		if locator.LineNumber != nil {
			ref["lineNumber"] = *locator.LineNumber
		}
		return ref
	}
	t.Fatal("the fixture graph carries no evidence record")
	return nil
}

// レコードへのメモは、根拠のレコードへ結んで返る。
func TestAssertionsEndpointRecordsTheNoteOnItsEvidenceRecord(t *testing.T) {
	handler := graphHandler(t)
	record := assertionRecordRefOf(t, handler)
	note := "接続先 port 5985 を WinRM と読んだ"
	created := decodeAssertionItem(t, requestAssertion(t, handler, http.MethodPost, assertionsPath,
		noteOnRecordBody(t, record, note)), http.StatusCreated)

	if created.Assertion.Target.Kind != core.AssertionTargetKindRecord {
		t.Fatalf("target kind=%q want %q", created.Assertion.Target.Kind,
			core.AssertionTargetKindRecord)
	}
	if created.Assertion.Basis.Note != note {
		t.Errorf("note=%q want %q", created.Assertion.Basis.Note, note)
	}
	// **所見は自動導出を上書きしない。** 対象の出所は観測のままである。
	if created.TargetOrigin != string(core.AssertionTargetOriginObservation) {
		t.Errorf("targetOrigin=%q want %q", created.TargetOrigin,
			core.AssertionTargetOriginObservation)
	}
	if len(created.Assertion.Basis.RecordRefs) != 1 {
		t.Errorf("the basis carries %d record refs, want the record the request named",
			len(created.Assertion.Basis.RecordRefs))
	}
}

// メモは新しい改訂で変えられ、置き換えられた改訂のメモは履歴に残る。
func TestAssertionRevisionChangesTheNoteAndKeepsTheSupersededOne(t *testing.T) {
	handler := graphHandler(t)
	record := assertionRecordRefOf(t, handler)
	created := decodeAssertionItem(t, requestAssertion(t, handler, http.MethodPost, assertionsPath,
		noteOnRecordBody(t, record, "最初のメモ")), http.StatusCreated)

	revised, err := json.Marshal(map[string]any{
		"target": created.Assertion.Target,
		"author": "analyst-b",
		"state":  string(core.AssertionStateActive),
		"basis":  map[string]any{"note": "読み直したメモ"},
		// 改訂は記録した最初の revision を元にする。
		"baseRevision": created.Assertion.RevisionNumber,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated := decodeAssertionItem(t, requestAssertion(t, handler, http.MethodPut,
		assertionsPath+"/"+created.Assertion.Id, string(revised)), http.StatusOK)

	if updated.Assertion.Basis.Note != "読み直したメモ" {
		t.Fatalf("the revised assertion carries the note %q, want the note of the request",
			updated.Assertion.Basis.Note)
	}
	if len(updated.Assertion.History) != 1 {
		t.Fatalf("the revised assertion carries %d superseded revisions, want 1",
			len(updated.Assertion.History))
	}
	if superseded := updated.Assertion.History[0].Basis.Note; superseded != "最初のメモ" {
		t.Errorf("the superseded revision carries the note %q, want the note of the first revision",
			superseded)
	}
}

// 識別子に該当する所見が無い改訂の追加は record_not_found を返す。
func TestAssertionRevisionReportsTheMissingAssertion(t *testing.T) {
	handler := graphHandler(t)
	encoded, err := json.Marshal(map[string]any{
		"target": map[string]any{
			"kind": "node", "nodeId": "n:process:absent",
		},
		"author": "analyst-a", "state": string(core.AssertionStateActive),
		"basis":        map[string]any{"note": "根拠"},
		"baseRevision": core.FirstAssertionRevisionNumber,
	})
	if err != nil {
		t.Fatal(err)
	}
	response := requestAssertion(t, handler, http.MethodPut, assertionsPath+"/as:absent",
		string(encoded))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d want %d body=%s", response.Code, http.StatusNotFound,
			response.Body.String())
	}
	var apiError core.ApiError
	decodeJSON(t, response.Body, &apiError)
	if apiError.Code != core.ApiErrorCodeRecordNotFound {
		t.Errorf("code = %q, want %q", apiError.Code, core.ApiErrorCodeRecordNotFound)
	}
}

// 記録の要求の綴り誤りと、定義の外の種別を退ける。
func TestAssertionsEndpointRejectsTheMalformedRequest(t *testing.T) {
	handler := graphHandler(t)
	graph := graphTargets(t, handler)
	observed := edgeOfState(t, graph, string(core.RelationStateObserved))

	cases := []struct {
		name string
		body string
	}{
		{name: "unknown item", body: `{"kind":"annotation","target":{"kind":"node","nodeId":"n:a"},` +
			`"author":"analyst-a","basis":{"note":"根拠"},"mode":"x"}`},
		{name: "unknown kind", body: `{"kind":"suspicion","target":{"kind":"node","nodeId":"n:a"},` +
			`"author":"analyst-a","basis":{"note":"根拠"}}`},
		{name: "target mixing references", body: `{"kind":"annotation","target":` +
			`{"kind":"node","nodeId":"n:a","record":{"sourceContentSha256":"` +
			strings.Repeat("0", 64) + `","positionKind":"line_number","lineNumber":1}},` +
			`"author":"analyst-a","basis":{"note":"根拠"}}`},
		{name: "state on a creation", body: edgeTargetBodyWithState(observed)},
		{name: "basis without a note", body: `{"kind":"annotation",` +
			`"target":{"kind":"node","nodeId":"n:a"},"author":"analyst-a","basis":{"note":""}}`},
		{name: "basis with a blank note", body: `{"kind":"annotation",` +
			`"target":{"kind":"node","nodeId":"n:a"},"author":"analyst-a","basis":{"note":" \t"}}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := requestAssertion(t, handler, http.MethodPost, assertionsPath, testCase.body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want %d body=%s", response.Code, http.StatusBadRequest,
					response.Body.String())
			}
			var apiError core.ApiError
			decodeJSON(t, response.Body, &apiError)
			if apiError.Code != core.ApiErrorCodeInvalidRequest {
				t.Errorf("code = %q, want %q", apiError.Code, core.ApiErrorCodeInvalidRequest)
			}
		})
	}
	accepted := requestAssertion(t, handler, http.MethodPost, assertionsPath,
		edgeTargetBody(observed, "根拠"))
	if accepted.Code != http.StatusCreated {
		t.Errorf("status=%d want %d for a well formed request body=%s",
			accepted.Code, http.StatusCreated, accepted.Body.String())
	}
}

// edgeTargetBodyWithState は、記録の要求に状態を載せた本文を組む。
func edgeTargetBodyWithState(edge graphEdge) string {
	body := map[string]any{
		"target": map[string]any{
			"kind": "edge",
			"edge": map[string]any{
				"kind": edge.Kind, "sourceNodeId": edge.SourceNodeId,
				"targetNodeId": edge.TargetNodeId,
			},
		},
		"author": "analyst-a",
		"state":  string(core.AssertionStateWithdrawn),
		"basis":  map[string]any{"note": "根拠"},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// 一覧は記録した所見を全件返す。
func TestAssertionsListingReturnsEveryRecordedAssertion(t *testing.T) {
	handler := graphHandler(t)
	graph := graphTargets(t, handler)
	observed := edgeOfState(t, graph, string(core.RelationStateObserved))
	recorded := make([]string, 0, 2)
	for _, note := range []string{"1 件目の根拠", "2 件目の根拠"} {
		item := decodeAssertionItem(t, requestAssertion(t, handler, http.MethodPost, assertionsPath,
			edgeTargetBody(observed, note)), http.StatusCreated)
		recorded = append(recorded, item.Assertion.Id)
	}

	listed := decodeAssertions(t, handler)
	if listed.AssertionCount != int64(len(listed.Assertions)) {
		t.Errorf("assertionCount = %d, assertions = %d, want the same number",
			listed.AssertionCount, len(listed.Assertions))
	}
	if len(listed.Assertions) != len(recorded) {
		t.Fatalf("the listing carries %+v, want the %d recorded assertions",
			listed.Assertions, len(recorded))
	}
	for _, id := range recorded {
		assertionItemOfId(t, listed, id)
	}
}

// **上限と続きを取る位置を受けない。** 綴りを通知せずに既定値で処理しないよう、未知の
// 項目として退ける。
func TestAssertionsListingRejectsALimitAndACursor(t *testing.T) {
	handler := graphHandler(t)
	for _, testCase := range []struct {
		name string
		path string
	}{
		{"limit", assertionsPath + "?limit=10"},
		{"cursor", assertionsPath + "?cursor=any-cursor"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := requestAssertion(t, handler, http.MethodGet, testCase.path, "")
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want %d body=%s", response.Code, http.StatusBadRequest,
					response.Body.String())
			}
			var apiError core.ApiError
			decodeJSON(t, response.Body, &apiError)
			if apiError.Code != core.ApiErrorCodeInvalidRequest {
				t.Fatalf("code=%q want invalid_request", apiError.Code)
			}
		})
	}
}

// 所見を 1 件も持たない一覧は、空である理由を持つ。
func TestAssertionsListingReportsTheEmptyReason(t *testing.T) {
	listed := decodeAssertions(t, graphHandler(t))
	if len(listed.Assertions) != 0 || listed.AssertionCount != 0 {
		t.Fatalf("the listing carries %+v, want it to be empty", listed.Assertions)
	}
	if listed.EmptyReason != core.EmptyReasonNoRecordInFilter {
		t.Errorf("emptyReason = %q, want %q", listed.EmptyReason,
			core.EmptyReasonNoRecordInFilter)
	}
}
