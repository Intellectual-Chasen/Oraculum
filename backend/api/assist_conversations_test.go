package api_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const conversationsPath = "/api/v0/conversations"

// assistSession は許可を記録して会話を発行した handler と会話の識別子である。
type assistSession struct {
	t       *testing.T
	handler http.Handler
	id      string
}

func openAssistSession(t *testing.T) assistSession {
	t.Helper()
	handler := assistHandlerOf(t)
	recordAssistPermission(t, handler, core.AssistPermissionActionGrant, http.StatusCreated)
	var opened struct {
		Conversation core.AssistConversation `json:"conversation"`
	}
	decodeInto(t, requestJSON(t, handler, http.MethodPost, conversationsPath,
		encodeBody(t, map[string]any{"provider": "claude", "model": "model-a"}), http.StatusCreated), &opened)
	if err := opened.Conversation.Validate(); err != nil {
		t.Fatal(err)
	}
	return assistSession{t: t, handler: handler, id: opened.Conversation.Id}
}

// post は会話の操作へ本文を送り、応答を返す。
func (s assistSession) post(operation string, body map[string]any) *httptest.ResponseRecorder {
	s.t.Helper()
	request := httptest.NewRequest(http.MethodPost, conversationsPath+"/"+s.id+"/"+operation,
		bytes.NewReader(encodeBody(s.t, body)))
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, request)
	return recorder
}

// expect は会話の操作の応答が期待した status であることを確かめ、本文を返す。
func (s assistSession) expect(operation string, body map[string]any, status int) []byte {
	s.t.Helper()
	response := s.post(operation, body)
	if response.Code != status {
		s.t.Fatalf("%s returned %d, want %d: %s", operation, response.Code, status, response.Body)
	}
	return response.Body.Bytes()
}

func (s assistSession) registerTurn(turnId string, extra map[string]any) []byte {
	s.t.Helper()
	body := map[string]any{"turnId": turnId, "matchConditions": []map[string]any{{"conditionKey": "destination_ip"}}}
	for key, value := range extra {
		body[key] = value
	}
	return s.expect("turns", body, http.StatusOK)
}

func (s assistSession) disclosures() []core.AssistDisclosure {
	s.t.Helper()
	var listed struct {
		Disclosures []core.AssistDisclosure `json:"disclosures"`
	}
	decodeInto(s.t, requestJSON(s.t, s.handler, http.MethodGet, "/api/v0/assist-disclosures?conversation="+s.id,
		nil, http.StatusOK), &listed)
	return listed.Disclosures
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func apiErrorOf(t *testing.T, body []byte) core.ApiError {
	t.Helper()
	var apiError core.ApiError
	decodeInto(t, body, &apiError)
	return apiError
}

// 受け渡しごとに監査記録が残り、記録の sha256 と長さは test が受け取った本文と一致する。
func TestAssistDisclosuresMatchTheSentBodies(t *testing.T) {
	session := openAssistSession(t)
	var bodies [][]byte
	bodies = append(bodies, session.registerTurn("turn-1", map[string]any{
		"searchQuery": map[string]any{"depth": 1, "valueContains": []string{"example"}},
	}))
	bodies = append(bodies, session.expect("overview", map[string]any{"turnId": "turn-1"}, http.StatusOK))
	search := session.expect("graph-search", map[string]any{
		"turnId": "turn-1", "searchQuery": map[string]any{"depth": 0, "nodeKinds": []string{"record"}},
	}, http.StatusOK)
	bodies = append(bodies, search)
	var found struct {
		Nodes []struct {
			Ref    string `json:"ref"`
			Record string `json:"record"`
		} `json:"nodes"`
		Records []any `json:"records"`
	}
	decodeInto(t, search, &found)
	if len(found.Nodes) == 0 || found.Nodes[0].Record == "" || !strings.HasPrefix(found.Nodes[0].Ref, "n") {
		t.Fatalf("graph search = %s, want record nodes with record references", search)
	}
	records := session.expect("records", map[string]any{
		"turnId": "turn-1", "refs": []string{found.Nodes[0].Record},
	}, http.StatusOK)
	bodies = append(bodies, records)
	var read struct {
		Records []struct {
			Ref     string `json:"ref"`
			RawText string `json:"rawText"`
		} `json:"records"`
	}
	decodeInto(t, records, &read)
	if len(read.Records) != 1 || read.Records[0].Ref != found.Nodes[0].Record || read.Records[0].RawText == "" {
		t.Fatalf("records = %s, want the raw text of the referenced record", records)
	}
	bodies = append(bodies, session.expect("targets", map[string]any{
		"turnId": "turn-1", "ref": found.Nodes[0].Ref,
	}, http.StatusOK))
	bodies = append(bodies, session.expect("timeline", map[string]any{
		"turnId": "turn-1", "conditions": map[string]any{},
	}, http.StatusOK))

	disclosures := session.disclosures()
	if len(disclosures) != len(bodies) {
		t.Fatalf("disclosures = %d, want one for each of the %d bodies", len(disclosures), len(bodies))
	}
	wantTools := []core.AssistTool{
		core.AssistToolTurn, core.AssistToolOverview, core.AssistToolGraphSearch, core.AssistToolRecords,
		core.AssistToolTarget, core.AssistToolTimeline,
	}
	for index, disclosure := range disclosures {
		if disclosure.Tool != wantTools[index] {
			t.Errorf("disclosure %d tool = %q, want %q", index, disclosure.Tool, wantTools[index])
		}
		if disclosure.BodySha256 != sha256Hex(bodies[index]) || disclosure.BodyBytes != int64(len(bodies[index])) {
			t.Errorf("disclosure %d = %s/%d, want the sha256 and the length of the sent body", index,
				disclosure.BodySha256, disclosure.BodyBytes)
		}
		if disclosure.Versions.PermissionRevision != 1 || len(disclosure.Versions.MatchConditions) != 1 {
			t.Errorf("disclosure %d versions = %+v", index, disclosure.Versions)
		}
	}
	// 記録の原文の受け渡しは、載せた記録の参照を数える。
	if got := disclosures[3].RecordRefs; len(got) != 1 || got[0] != found.Nodes[0].Record {
		t.Errorf("records disclosure refs = %v", got)
	}
	// 要求の本文は、証拠の値を含む検索の条件ごと記録に残る。
	if !strings.Contains(disclosures[0].Request, "example") {
		t.Errorf("turn request = %s, want the search query", disclosures[0].Request)
	}
}

// LLM へ渡す発言の本文は、画面の図が出していた対象と、対象を検索の条件から決めていたことを持つ。
func TestAssistTurnPassesTheShownNodeKinds(t *testing.T) {
	session := openAssistSession(t)
	body := session.registerTurn("turn-1", map[string]any{
		"searchQuery":             map[string]any{"depth": 1, "nodeKinds": []string{"terminal"}, "granularity": "object"},
		"nodeKindsFromConditions": true,
	})
	var registered struct {
		SearchQuery             core.SearchQuery `json:"searchQuery"`
		NodeKindsFromConditions bool             `json:"nodeKindsFromConditions"`
	}
	decodeInto(t, body, &registered)
	kinds := registered.SearchQuery.NodeKinds
	if len(kinds) != 1 || kinds[0] != core.NodeKindTerminal || registered.SearchQuery.Granularity != "object" ||
		!registered.NodeKindsFromConditions {
		t.Errorf("turn body = %s, want the shown node kinds chosen from the conditions", body)
	}
}

// 許可の無い調査は会話を発行せず、会話の途中で取り消すと以後の受け渡しを退ける。
func TestAssistConversationFollowsThePermission(t *testing.T) {
	handler := assistHandlerOf(t)
	refused := requestJSON(t, handler, http.MethodPost, conversationsPath,
		encodeBody(t, map[string]any{"provider": "claude"}), http.StatusConflict)
	if code := apiErrorOf(t, refused).Code; code != core.ApiErrorCodeAssistNotPermitted {
		t.Errorf("open without a permission = %q, want assist_not_permitted", code)
	}

	session := openAssistSession(t)
	session.registerTurn("turn-1", nil)
	recordAssistPermission(t, session.handler, core.AssistPermissionActionRevoke, http.StatusCreated)
	before := len(session.disclosures())
	response := session.expect("overview", map[string]any{"turnId": "turn-1"}, http.StatusConflict)
	if code := apiErrorOf(t, response).Code; code != core.ApiErrorCodeAssistNotPermitted {
		t.Errorf("overview after the revocation = %q, want assist_not_permitted", code)
	}
	if after := len(session.disclosures()); after != before {
		t.Errorf("the refused read left a disclosure: %d, want %d", after, before)
	}
	// 検索の条件の検証も退ける。検証の理由は案件と端末が実在するかを伝える。
	checked := session.expect("search-query-checks", map[string]any{
		"turnId": "turn-1", "searchQuery": map[string]any{"depth": 1},
	}, http.StatusConflict)
	if code := apiErrorOf(t, checked).Code; code != core.ApiErrorCodeAssistNotPermitted {
		t.Errorf("search query check after the revocation = %q, want assist_not_permitted", code)
	}
}

// グラフの検索の起点は、この会話で発行したノードの短い参照で指す。
func TestAssistGraphSearchStartsFromIssuedNodeReferences(t *testing.T) {
	session := openAssistSession(t)
	session.registerTurn("turn-1", nil)
	var first struct {
		Nodes []struct {
			Ref string `json:"ref"`
		} `json:"nodes"`
	}
	decodeInto(t, session.expect("graph-search", map[string]any{
		"turnId": "turn-1", "searchQuery": map[string]any{"depth": 0, "nodeKinds": []string{"process"}},
	}, http.StatusOK), &first)
	if len(first.Nodes) == 0 {
		t.Fatal("the graph search returned no process node")
	}
	var around struct {
		MatchedNodeCount int64 `json:"matchedNodeCount"`
		Nodes            []struct {
			Ref string `json:"ref"`
		} `json:"nodes"`
	}
	decodeInto(t, session.expect("graph-search", map[string]any{
		"turnId": "turn-1", "searchQuery": map[string]any{"depth": 1, "nodeIds": []string{first.Nodes[0].Ref}},
	}, http.StatusOK), &around)
	if around.MatchedNodeCount == 0 || around.Nodes[0].Ref != first.Nodes[0].Ref {
		t.Errorf("search from %s = %+v, want the origin node and its neighbours", first.Nodes[0].Ref, around)
	}
	for name, origin := range map[string]string{"unissued": "n99", "record reference": "r1", "raw": "n:process:1"} {
		response := session.expect("graph-search", map[string]any{
			"turnId": "turn-1", "searchQuery": map[string]any{"depth": 1, "nodeIds": []string{origin}},
		}, http.StatusBadRequest)
		if code := apiErrorOf(t, response).Code; code != core.ApiErrorCodeInvalidRequest {
			t.Errorf("%s origin: code = %q, want invalid_request", name, code)
		}
	}
}

// 発行していない参照、登録していない発言、知らない会話を退ける。
func TestAssistConversationRejectsUnknownReferences(t *testing.T) {
	session := openAssistSession(t)
	session.registerTurn("turn-1", nil)
	for name, request := range map[string]struct {
		operation string
		body      map[string]any
		status    int
		code      core.ApiErrorCode
	}{
		"record not issued": {"records", map[string]any{"turnId": "turn-1", "refs": []string{"r99"}},
			http.StatusBadRequest, core.ApiErrorCodeInvalidRequest},
		"node not issued": {"targets", map[string]any{"turnId": "turn-1", "ref": "n99"},
			http.StatusBadRequest, core.ApiErrorCodeInvalidRequest},
		"turn not registered": {"overview", map[string]any{"turnId": "turn-9"},
			http.StatusBadRequest, core.ApiErrorCodeInvalidRequest},
		"turn registered twice": {"turns", map[string]any{
			"turnId": "turn-1", "matchConditions": []map[string]any{{"conditionKey": "destination_ip"}},
		}, http.StatusBadRequest, core.ApiErrorCodeInvalidRequest},
	} {
		response := session.expect(request.operation, request.body, request.status)
		if code := apiErrorOf(t, response).Code; code != request.code {
			t.Errorf("%s: code = %q, want %q", name, code, request.code)
		}
	}
	unknown := assistSession{t: t, handler: session.handler, id: strings.Repeat("f", 32)}
	response := unknown.expect("overview", map[string]any{"turnId": "turn-1"}, http.StatusNotFound)
	if code := apiErrorOf(t, response).Code; code != core.ApiErrorCodeConversationNotFound {
		t.Errorf("unknown conversation = %q, want conversation_not_found", code)
	}
	if got := len(session.disclosures()); got != 1 {
		t.Errorf("disclosures = %d, want only the turn", got)
	}
}

// 検索の条件の検証は受理か理由だけを返し、受け渡しを記録しない。
func TestAssistSearchQueryCheckRecordsNothing(t *testing.T) {
	session := openAssistSession(t)
	session.registerTurn("turn-1", nil)
	for name, check := range map[string]struct {
		query    map[string]any
		accepted bool
	}{
		"valid":           {map[string]any{"depth": 1, "nodeKinds": []string{"process"}}, true},
		"too deep":        {map[string]any{"depth": 99}, false},
		"unknown case":    {map[string]any{"depth": 0, "case": "case-absent"}, false},
		"field only":      {map[string]any{"depth": 0, "valueField": "f"}, false},
		"unknown kind":    {map[string]any{"depth": 0, "nodeKinds": []string{"planet"}}, false},
		"address as text": {map[string]any{"depth": 0, "addressInCidr": "198.51.100.1"}, false},
		"unissued origin": {map[string]any{"depth": 1, "nodeIds": []string{"n:1"}}, false},
		"record granularity without the record kind": {map[string]any{"depth": 1, "granularity": "record"},
			false},
		"search expression": {map[string]any{"depth": 1, "searchExpression": `process.name == "cmd.exe"`},
			true},
		// 構文の誤った検索式の card を画面に出すと、適用した画面の要求が退けられる。
		"broken search expression": {map[string]any{"depth": 1, "searchExpression": "process.name =="}, false},
		"unknown source":           {map[string]any{"depth": 1, "sources": []string{"src-absent"}}, false},
	} {
		var result struct {
			Accepted bool   `json:"accepted"`
			Reason   string `json:"reason"`
		}
		body := session.expect("search-query-checks", map[string]any{"turnId": "turn-1", "searchQuery": check.query},
			http.StatusOK)
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}
		if result.Accepted != check.accepted || (!check.accepted && result.Reason == "") {
			t.Errorf("%s: result = %+v, want accepted=%v with a reason when rejected", name, result, check.accepted)
		}
	}
	if got := len(session.disclosures()); got != 1 {
		t.Errorf("disclosures = %d, want only the turn", got)
	}
}

// 検索の条件の検証は、起点の短い参照をノードの識別子へ直し、種別と表示名を返す。画面の文脈の
// 起点は、LLM へ渡す本文で短い参照へ直す。
func TestAssistSearchQueryCheckResolvesOrigins(t *testing.T) {
	session := openAssistSession(t)
	session.registerTurn("turn-1", nil)
	var found struct {
		Nodes []struct {
			Ref   string        `json:"ref"`
			Kind  core.NodeKind `json:"kind"`
			Label string        `json:"label"`
		} `json:"nodes"`
	}
	decodeInto(t, session.expect("graph-search", map[string]any{
		"turnId": "turn-1", "searchQuery": map[string]any{"depth": 0, "nodeKinds": []string{"process"}},
	}, http.StatusOK), &found)
	if len(found.Nodes) == 0 {
		t.Fatal("the graph search returned no process node")
	}
	origin := found.Nodes[0]
	var checked struct {
		Accepted bool                `json:"accepted"`
		Origins  []core.AssistOrigin `json:"origins"`
	}
	decodeInto(t, session.expect("search-query-checks", map[string]any{
		"turnId": "turn-1", "searchQuery": map[string]any{"depth": 2, "nodeIds": []string{origin.Ref}},
	}, http.StatusOK), &checked)
	if !checked.Accepted || len(checked.Origins) != 1 || checked.Origins[0].Kind != origin.Kind ||
		checked.Origins[0].Label != origin.Label || checked.Origins[0].Id == "" || checked.Origins[0].Id == origin.Ref {
		t.Fatalf("check of %s = %+v, want the node id, kind and label", origin.Ref, checked)
	}
	nodeId := checked.Origins[0].Id

	// 発行した関係の参照は起点に使えない。
	var around struct {
		Edges []struct {
			Ref string `json:"ref"`
		} `json:"edges"`
	}
	decodeInto(t, session.expect("graph-search", map[string]any{
		"turnId": "turn-1", "searchQuery": map[string]any{"depth": 1, "nodeIds": []string{origin.Ref}},
	}, http.StatusOK), &around)
	if len(around.Edges) == 0 {
		t.Fatal("the graph search from the origin returned no edge")
	}
	var refused struct {
		Accepted bool   `json:"accepted"`
		Reason   string `json:"reason"`
	}
	decodeInto(t, session.expect("search-query-checks", map[string]any{
		"turnId": "turn-1", "searchQuery": map[string]any{"depth": 1, "nodeIds": []string{around.Edges[0].Ref}},
	}, http.StatusOK), &refused)
	if refused.Accepted || refused.Reason == "" {
		t.Errorf("check from the edge %s = %+v, want a refusal with a reason", around.Edges[0].Ref, refused)
	}

	body := session.registerTurn("turn-2", map[string]any{
		"searchQuery": map[string]any{"depth": 2, "nodeIds": []string{nodeId}},
	})
	var registered struct {
		SearchQuery core.SearchQuery `json:"searchQuery"`
		Nodes       []struct {
			Ref string `json:"ref"`
		} `json:"nodes"`
	}
	decodeInto(t, body, &registered)
	if strings.Contains(string(body), nodeId) || len(registered.SearchQuery.NodeIds) != 1 ||
		registered.SearchQuery.NodeIds[0] != origin.Ref {
		t.Errorf("turn body = %s, want the origin as %s without the node id", body, origin.Ref)
	}
	if len(registered.Nodes) != 1 || registered.Nodes[0].Ref != origin.Ref {
		t.Errorf("turn nodes = %+v, want the origin summary", registered.Nodes)
	}
	response := session.expect("turns", map[string]any{
		"turnId": "turn-3", "matchConditions": []map[string]any{{"conditionKey": "destination_ip"}},
		"searchQuery": map[string]any{"depth": 1, "nodeIds": []string{"n:absent"}},
	}, http.StatusNotFound)
	if code := apiErrorOf(t, response).Code; code != core.ApiErrorCodeRecordNotFound {
		t.Errorf("turn with an absent origin = %q, want record_not_found", code)
	}
}

// 調査の directory を渡さない起動は、会話の発行と監査記録の閲覧を assist_unavailable で退ける。
func TestAssistConversationIsUnavailableWithoutAnInvestigation(t *testing.T) {
	handler := testHandler(graphImportResult(t))
	response := requestJSON(t, handler, http.MethodPost, conversationsPath,
		encodeBody(t, map[string]any{"provider": "claude"}), http.StatusConflict)
	if code := apiErrorOf(t, response).Code; code != core.ApiErrorCodeAssistUnavailable {
		t.Errorf("open = %q, want assist_unavailable", code)
	}
	response = requestJSON(t, handler, http.MethodGet, "/api/v0/assist-disclosures?conversation="+strings.Repeat("a", 32),
		nil, http.StatusConflict)
	if code := apiErrorOf(t, response).Code; code != core.ApiErrorCodeAssistUnavailable {
		t.Errorf("disclosures = %q, want assist_unavailable", code)
	}
}
