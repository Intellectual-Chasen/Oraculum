package api_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/attackrules"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const attackCandidatesPath = "/api/v0/attack-candidates"

type attackCandidatesResponse struct {
	RuleSet struct {
		Directory      string `json:"directory"`
		FileCount      int    `json:"fileCount"`
		ContentSha256  string `json:"contentSha256"`
		Revision       string `json:"revision"`
		RevisionSource string `json:"revisionSource"`
	} `json:"ruleSet"`
	Rules []struct {
		ID                string   `json:"id"`
		Title             string   `json:"title"`
		Description       string   `json:"description"`
		References        []string `json:"references"`
		DistinguishesFrom []string `json:"distinguishesFrom"`
		MatchCount        int64    `json:"matchCount"`
		Attack            []struct {
			ID    string `json:"id"`
			Basis string `json:"basis"`
		} `json:"attack"`
		Variants []struct {
			ID         string   `json:"id"`
			Platforms  []string `json:"platforms"`
			Rationale  string   `json:"rationale"`
			MatchCount int64    `json:"matchCount"`
			Evaluation struct {
				State  string `json:"state"`
				Reason string `json:"reason"`
				Detail string `json:"detail"`
			} `json:"evaluation"`
		} `json:"variants"`
	} `json:"rules"`
	Matches      []attackCandidateMatchItem `json:"matches"`
	NotEvaluated []struct {
		RuleID    string `json:"ruleId"`
		VariantID string `json:"variantId"`
		Reason    string `json:"reason"`
		Detail    string `json:"detail"`
	} `json:"notEvaluated"`
}

type attackCandidateMatchItem struct {
	RuleID    string `json:"ruleId"`
	VariantID string `json:"variantId"`
	MatchID   string `json:"matchId"`
	Edges     []struct {
		Role            string                     `json:"role"`
		EvidenceRole    string                     `json:"evidenceRole"`
		Kind            core.EdgeKind              `json:"kind"`
		EdgeID          string                     `json:"edgeId"`
		SourceNode      core.GraphNode             `json:"sourceNode"`
		SinkNode        core.GraphNode             `json:"sinkNode"`
		AssignmentBases []core.EdgeAssignmentBasis `json:"assignmentBases"`
		Evidence        []core.RecordLocator       `json:"evidence"`
	} `json:"edges"`
}

func attackCandidatesStore(t *testing.T) *pipeline.MemoryStore {
	t.Helper()
	contents := map[string]string{
		"baseline.log": "02/01/2000 03:04:07.890 +0900 sn=11 evt=ps subEvt=inject tmid=T1 psGUID={P1} tpsGUID={P9}\n" +
			"02/01/2000 03:04:08.890 +0900 sn=12 evt=ps subEvt=inject tmid=T1 psGUID={P2} tpsGUID={P8}\n",
		"challenge.log": "02/01/2000 03:04:09.890 +0900 sn=13 evt=ps subEvt=inject tmid=T2 psGUID={P3} tpsGUID={P7}\n",
		"session.log": "02/01/2000 03:04:10.890 +0900 sn=14 evt=ps subEvt=start psGUID={S1} tmid=T3 com=HOST3 csid=S-3 ip=192.0.2.30 psPath=app.exe\n" +
			"02/01/2000 03:04:11.890 +0900 sn=15 evt=ps subEvt=start psGUID={S2} tmid=T4 com=HOST4 csid=S-4 ip=192.0.2.40 psPath=app.exe\n" +
			"02/01/2000 03:04:12.890 +0900 sn=16 evt=session subEvt=loginR tmid=T4 com=HOST4 csid=S-4 ip=192.0.2.40 srcIP=192.0.2.30 srcPort=50001\n",
	}
	runner, err := pipeline.NewRunner(pipeline.Config{
		Open:    func(path string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(contents[path])), nil },
		Parsers: testFormatRegistry(t), Minter: pipeline.DigestMinter{},
		Ordinals: pipeline.NewInMemoryOrdinals(), Sanitize: output.Sanitize,
	})
	if err != nil {
		t.Fatal(err)
	}
	baseline, challenge := "baseline", "challenge"
	result, err := runner.Run([]pipeline.SourcePlan{
		{OriginPath: "baseline.log", FileName: "baseline.log", FormatKey: markIIFormatKey, CaseId: &baseline},
		{OriginPath: "challenge.log", FileName: "challenge.log", FormatKey: markIIFormatKey, CaseId: &challenge},
		{OriginPath: "session.log", FileName: "session.log", FormatKey: markIIFormatKey, CaseId: &challenge},
	})
	if err != nil {
		t.Fatal(err)
	}
	return pipeline.NewMemoryStore(result, &testAssertionClock{})
}

func requestAttackCandidates(t *testing.T, handler http.Handler, method, query string, status int) attackCandidatesResponse {
	t.Helper()
	path := attackCandidatesPath + "?" + wholeGraphQuery
	if query != "" {
		path += "&" + query
	}
	response := requestPath(t, handler, method, withAllMatchConditions(path))
	if response.Code != status {
		t.Fatalf("status=%d want=%d body=%s", response.Code, status, response.Body.String())
	}
	if status != http.StatusOK {
		if status == http.StatusMethodNotAllowed {
			if response.Header().Get("Allow") != "GET, HEAD" {
				t.Fatalf("Allow=%q", response.Header().Get("Allow"))
			}
			return attackCandidatesResponse{}
		}
		var problem core.ApiError
		decodeJSON(t, response.Body, &problem)
		if problem.Code != core.ApiErrorCodeInvalidRequest {
			t.Fatalf("error=%+v", problem)
		}
		return attackCandidatesResponse{}
	}
	var payload attackCandidatesResponse
	decodeJSON(t, response.Body, &payload)
	return payload
}

func TestAttackCandidatesReturnsDirectedMatchesAndPreservesAssertions(t *testing.T) {
	store := attackCandidatesStore(t)
	if _, err := store.Assertions().Create(pipeline.AssertionDraft{
		Target: core.AssertionTarget{
			Kind: core.AssertionTargetKindNode, NodeId: "n:process:existing",
		},
		Author: "analyst",
		Basis:  core.AssertionBasis{Note: "候補とは独立した既存所見"},
	}); err != nil {
		t.Fatal(err)
	}
	catalog := pipeline.NewGraphCatalog(store, pipeline.DefaultGraphLayers())
	beforeGraph, _, err := catalog.Graph(context.Background(), pipeline.AllMatchConditions())
	if err != nil {
		t.Fatal(err)
	}
	ruleSet, err := pipeline.LoadAttackRuleSet("../rules/attack")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := api.NewHandler(store, catalog, pipeline.SigmaEvaluation{}, ruleSet)
	if err != nil {
		t.Fatal(err)
	}
	before := store.Assertions().List()
	got := requestAttackCandidates(t, handler, http.MethodGet, "", http.StatusOK)
	rulesByID := make(map[string]int64, len(got.Rules))
	hasRemoteServices := false
	for _, rule := range got.Rules {
		if _, exists := rulesByID[rule.ID]; exists {
			t.Fatalf("duplicate rule ID %q", rule.ID)
		}
		rulesByID[rule.ID] = rule.MatchCount
		if rule.ID == "attack.t1021.remote-services" {
			hasRemoteServices = true
			if rule.Attack[0].ID != "T1021" || rule.Attack[0].Basis != "inferred" || rule.Title != "Remote Services" ||
				rule.Description != "観測された遠隔セッションと接続元アドレスの対応を確認する候補。" {
				t.Fatalf("remote services rule=%+v", rule)
			}
		}
	}
	if !hasRemoteServices {
		t.Fatalf("rules/matches=%+v", got)
	}
	if _, hasProcessInjection := rulesByID["attack.t1055.process-injection"]; !hasProcessInjection {
		t.Fatalf("rules/matches=%+v", got)
	}
	matchCountsByRuleID := make(map[string]int64, len(rulesByID))
	matchIDsByRule := make(map[string]map[string]struct{}, len(rulesByID))
	for _, match := range got.Matches {
		matchCountsByRuleID[match.RuleID]++
		if matchIDsByRule[match.RuleID] == nil {
			matchIDsByRule[match.RuleID] = make(map[string]struct{})
		}
		if _, exists := matchIDsByRule[match.RuleID][match.MatchID]; exists {
			t.Fatalf("rule %q returned duplicate matchId %q", match.RuleID, match.MatchID)
		}
		matchIDsByRule[match.RuleID][match.MatchID] = struct{}{}
	}
	for ruleID, reportedCount := range rulesByID {
		if reportedCount != matchCountsByRuleID[ruleID] {
			t.Fatalf("rule %q reports %d matches, response contains %d",
				ruleID, reportedCount, matchCountsByRuleID[ruleID])
		}
	}
	for ruleID := range matchCountsByRuleID {
		if _, exists := rulesByID[ruleID]; !exists {
			t.Fatalf("match refers to unknown rule %q", ruleID)
		}
	}
	if len(matchIDsByRule["attack.t1021.remote-services"]) != 1 {
		t.Fatalf("T1021 combinations=%+v, want one matchId", matchIDsByRule["attack.t1021.remote-services"])
	}
	want := map[string]struct {
		sink, file string
		line       int64
	}{
		"{P1}": {"{P9}", "baseline.log", 1},
		"{P2}": {"{P8}", "baseline.log", 2},
		"{P3}": {"{P7}", "challenge.log", 1},
	}
	seen := map[string]bool{}
	for _, match := range got.Matches {
		if match.MatchID == "" || len(match.Edges) == 0 {
			t.Fatalf("invalid match identity or empty edges=%+v", match)
		}
		if match.RuleID == "attack.t1021.remote-services" {
			if len(match.Edges) != 2 || match.Edges[0].Role != "address" || match.Edges[1].Role != "session" {
				t.Fatalf("invalid remote service match=%+v", match)
			}
			address, session := match.Edges[0], match.Edges[1]
			if session.EdgeID == "" || address.EdgeID == "" || session.EdgeID == address.EdgeID ||
				session.Kind != core.EdgeKindTerminalRemoteSession || address.Kind != core.EdgeKindTerminalAddress ||
				session.SourceNode.Kind != core.NodeKindTerminal || session.SinkNode.Kind != core.NodeKindTerminal ||
				len(session.SourceNode.Identity) != 1 || session.SourceNode.Identity[0].Semantic != core.SemanticKeyTerminalId ||
				session.SourceNode.Identity[0].Value != "T3" ||
				len(session.SinkNode.Identity) != 1 || session.SinkNode.Identity[0].Semantic != core.SemanticKeyTerminalId ||
				session.SinkNode.Identity[0].Value != "T4" ||
				len(session.Evidence) != 1 || session.Evidence[0].LineNumber == nil || *session.Evidence[0].LineNumber != 3 ||
				session.Evidence[0].SourceFileName != "session.log" ||
				len(address.Evidence) != 1 || address.Evidence[0].LineNumber == nil ||
				*address.Evidence[0].LineNumber != 1 || address.Evidence[0].SourceFileName != "session.log" ||
				address.SourceNode.Kind != core.NodeKindTerminal || address.SinkNode.Kind != core.NodeKindIp ||
				len(session.AssignmentBases) != 1 || address.AssignmentBases == nil || len(address.AssignmentBases) != 0 {
				t.Fatalf("invalid remote service edges=%+v", match.Edges)
			}
			clientIP := ""
			for _, identity := range address.SinkNode.Identity {
				clientIP = identity.Value
			}
			if clientIP == "" || session.AssignmentBases[0].ClientIp != clientIP {
				t.Fatalf("session assignment bases=%+v, address client IP=%q", session.AssignmentBases, clientIP)
			}
			continue
		}
		if len(match.Edges) != 1 || match.Edges[0].Role != "injection" {
			t.Fatalf("invalid process injection match=%+v", match)
		}
		edge := match.Edges[0]
		expected, found := want[edge.SourceNode.Identity[1].Value]
		if match.RuleID != "attack.t1055.process-injection" || edge.EdgeID == "" || seen[edge.EdgeID] ||
			edge.Kind != core.EdgeKindProcessInjection ||
			!found ||
			edge.SourceNode.Kind != core.NodeKindProcess || edge.SinkNode.Kind != core.NodeKindProcess ||
			edge.SinkNode.Identity[1].Value != expected.sink || edge.AssignmentBases == nil || len(edge.Evidence) != 1 ||
			edge.Evidence[0].LineNumber == nil || *edge.Evidence[0].LineNumber != expected.line ||
			edge.Evidence[0].SourceFileName != expected.file {
			t.Fatalf("invalid match=%+v", match)
		}
		seen[edge.EdgeID] = true
	}
	if after := store.Assertions().List(); !reflect.DeepEqual(before, after) {
		t.Fatalf("assertions changed: before=%+v after=%+v", before, after)
	}
	afterGraph, _, err := catalog.Graph(context.Background(), pipeline.AllMatchConditions())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeGraph, afterGraph) {
		t.Fatal("graph changed while serving attack candidates")
	}
}

func TestAttackCandidatesFollowGraphScopeAndErrors(t *testing.T) {
	handler := handlerOf(attackCandidatesStore(t))
	all := requestAttackCandidates(t, handler, http.MethodGet, "", http.StatusOK)
	byNode := requestAttackCandidates(t, handler, http.MethodGet,
		"nodeId="+url.QueryEscape(all.Matches[0].Edges[0].SourceNode.Id), http.StatusOK)
	if len(byNode.Matches) != 1 || byNode.Matches[0].MatchID != all.Matches[0].MatchID {
		t.Fatalf("node selection returned %+v, want match %q", byNode.Matches, all.Matches[0].MatchID)
	}
	for _, testCase := range []struct {
		name, query string
		want        int
	}{
		{"baseline case", "case=baseline", 2},
		{"challenge case", "case=challenge", 2},
		{"no event", "eventCategory=file", 0},
		{"event action", "eventAction=inject", 3},
		{"no action", "eventAction=create", 0},
		{"other edge", "edgeKind=ran_on", 0},
		{"depth zero", "depth=0", 0},
		{"value absent", "valueContains=unfindable", 0},
		{"future period", "timeFrom=2030-01-01T00:00:00Z&timeFromPrecision=second&filterUnit=second", 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			query := testCase.query
			if strings.HasPrefix(query, "depth=") {
				path := attackCandidatesPath + "?depth=0"
				response := requestPath(t, handler, http.MethodGet, withAllMatchConditions(path))
				if response.Code != http.StatusOK {
					t.Fatal(response.Body.String())
				}
				var got attackCandidatesResponse
				decodeJSON(t, response.Body, &got)
				if len(got.Matches) != 0 {
					t.Fatalf("matches=%+v", got.Matches)
				}
				return
			}
			got := requestAttackCandidates(t, handler, http.MethodGet, query, http.StatusOK)
			if len(got.Matches) != testCase.want || len(got.Rules) != 2 {
				t.Fatalf("query=%q payload=%+v", query, got)
			}
		})
	}
	for _, query := range []string{"case=missing", "depth=bad", "unexpected=value"} {
		requestAttackCandidates(t, handler, http.MethodGet, query, http.StatusBadRequest)
	}
	for _, query := range []string{
		attackCandidatesPath + "?" + wholeGraphQuery,
		attackCandidatesPath + "?" + wholeGraphQuery + "&matchCondition=unknown",
	} {
		response := requestPath(t, handler, http.MethodGet, query)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("query=%q status=%d body=%s", query, response.Code, response.Body.String())
		}
		var problem core.ApiError
		decodeJSON(t, response.Body, &problem)
		if problem.Code != core.ApiErrorCodeInvalidRequest {
			t.Fatalf("problem=%+v", problem)
		}
	}
	requestAttackCandidates(t, handler, http.MethodGet, "terminal=n:terminal:absent", http.StatusBadRequest)
	requestAttackCandidates(t, handler, http.MethodPost, "", http.StatusMethodNotAllowed)
}

func TestAttackCandidatesDistinguishesMissingInputFromZeroMatches(t *testing.T) {
	store := attackCandidatesStore(t)
	catalog := pipeline.NewGraphCatalog(store, pipeline.DefaultGraphLayers())
	set := pipeline.AttackRuleSet{
		Info: pipeline.AttackRuleSetInfo{
			Directory: "/rules/test", FileCount: 1, ContentSha256: strings.Repeat("a", 64),
			Revision: "synthetic", RevisionSource: "test",
		},
		Rules: []attackrules.Rule{{
			ID: "synthetic.input-required", Title: "Input required", Description: "Requires observed Sysmon input.",
			References: []string{"https://example.test/rules/input-required"},
			Attack:     []attackrules.AttackRef{{ID: "T1055", Basis: attackrules.AttackBasisInferred}},
			Variants: []attackrules.Variant{{ID: "sysmon", Pattern: attackrules.Pattern{
				Inputs: []string{attackrules.InputWindowsSysmon},
				Nodes: map[string]attackrules.PatternNode{
					"source": {Kind: string(core.NodeKindProcess)},
					"target": {Kind: string(core.NodeKindProcess)},
				},
				Edges: map[string]attackrules.PatternEdge{
					"injection": {Kind: string(core.EdgeKindProcessInjection), From: "source", To: "target"},
				},
				Evidence: attackrules.Evidence{Required: []string{"injection"}},
			}}},
		}},
	}
	handler, err := api.NewHandler(store, catalog, pipeline.SigmaEvaluation{}, set)
	if err != nil {
		t.Fatal(err)
	}
	got := requestAttackCandidates(t, handler, http.MethodGet, "", http.StatusOK)
	if len(got.Rules) != 1 || got.Rules[0].MatchCount != 0 || len(got.Rules[0].Variants) != 1 ||
		got.Rules[0].Variants[0].Evaluation.State != "not_evaluated" ||
		got.Rules[0].Variants[0].Evaluation.Reason != "missing_input" ||
		len(got.NotEvaluated) != 1 || got.NotEvaluated[0].Reason != "missing_input" || len(got.Matches) != 0 {
		t.Fatalf("candidate response = %+v; want missing_input, not evaluated zero matches", got)
	}
	// 規則が持たない一覧は、画面が配列として読めるように空の配列で返す。
	body := requestPath(t, handler, http.MethodGet, withAllMatchConditions(attackCandidatesPath+"?"+wholeGraphQuery)).Body.String()
	for _, want := range []string{`"distinguishesFrom":[]`, `"platforms":[]`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %s: %s", want, body)
		}
	}
}

func TestAttackCandidateCountsDoNotDependOnDisplayExpansion(t *testing.T) {
	handler := handlerOf(attackCandidatesStore(t))
	var matchesByDisplay [][]string
	displays := []string{"", "&nodeLimit=1", "&granularity=object"}
	for _, display := range displays {
		path := attackCandidatesPath + "?depth=1" + display
		response := requestPath(t, handler, http.MethodGet, withAllMatchConditions(path))
		if response.Code != http.StatusOK {
			t.Fatalf("%q: status=%d body=%s", display, response.Code, response.Body.String())
		}
		var got attackCandidatesResponse
		decodeJSON(t, response.Body, &got)
		if len(got.Rules) != 2 || got.Rules[0].MatchCount+got.Rules[1].MatchCount != int64(len(got.Matches)) {
			t.Fatalf("%q: rules/matches=%+v", display, got)
		}
		ids := make([]string, 0, len(got.Matches))
		for _, match := range got.Matches {
			ids = append(ids, match.MatchID)
		}
		matchesByDisplay = append(matchesByDisplay, ids)
	}
	if len(matchesByDisplay[0]) == 0 {
		t.Fatal("the fixture carries no candidate")
	}
	for i, ids := range matchesByDisplay[1:] {
		if !reflect.DeepEqual(matchesByDisplay[0], ids) {
			t.Fatalf("display %q changed candidates: %v, want %v", displays[i+1], ids, matchesByDisplay[0])
		}
	}
	response := requestPath(t, handler, http.MethodGet,
		withAllMatchConditions(attackCandidatesPath+"?expansion=unfolded&depth=1"))
	if response.Code != http.StatusBadRequest {
		t.Errorf("the removed expansion answers %d, want 400", response.Code)
	}
}
