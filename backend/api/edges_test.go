package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// edgesPath は`/api/v0/edges/{id}` の path の接頭辞である。
const edgesPath = "/api/v0/edges/"

// edgeResponse は`/api/v0/edges/{id}` の応答の項目名を test 側で固定する。decodeJSON が
// DisallowUnknownFields で読むため、handler が項目を足すと test が失敗する。
type edgeResponse struct {
	Edge       edgeDetailEdge `json:"edge"`
	SourceNode subgraphNode   `json:"sourceNode"`
	TargetNode subgraphNode   `json:"targetNode"`
	// MatchRecords と MatchStages と Matches は関連付けの表である。関連付けは expandedMatches で組み直す。
	MatchRecords []core.EdgeMatchRecord `json:"matchRecords"`
	MatchStages  []core.EdgeMatchStage  `json:"matchStages"`
	Matches      []core.EdgeMatchRef    `json:"matches"`
	MatchCount   int64                  `json:"matchCount"`
	// AssignmentBases は候補のエッジの成立の根拠である。出ないエッジでは要素数 0 になる。
	AssignmentBases []core.EdgeAssignmentBasis `json:"assignmentBases"`
	// TerminalAssignments はエッジを作った端末の割当である。出ないエッジでは要素数 0 になる。
	TerminalAssignments []core.TerminalAssignment `json:"terminalAssignments"`
	// ObservedInRecords は割当を持つエッジをレコードも直に観測したかである。
	// terminalAssignments と同時に出る。
	ObservedInRecords *bool `json:"observedInRecords"`
	// RecordPairs と RecordPairCount は観測の層が候補のエッジを作ったレコードの組である。
	// 組を持たないエッジでは出ない。
	RecordPairs     []core.EdgeRecordPair `json:"recordPairs"`
	RecordPairCount *int64                `json:"recordPairCount"`
	// EvidenceGroups は根拠を観測の種別と接続先 port で分けた区分である。
	EvidenceGroups []core.EdgeEvidenceGroup `json:"evidenceGroups"`
}

// edgeDetailEdge は`/api/v0/edges/{id}` だけが持つ根拠を足したエッジである。`/api/v0/graph` は根拠を持たない。
type edgeDetailEdge struct {
	graphEdge
	Evidence []core.GraphEvidence `json:"evidence"`
}

// expandedMatches は応答の関連付けの表を確かめ、関連付けを並び順のまま組み直す。
//
// 組み直した関連付けは、関連付けの結果 1 件の項目の名前を test 側で固定した edgeMatch へ JSON を通して読む。
func (r edgeResponse) expandedMatches(t *testing.T) []edgeMatch {
	t.Helper()
	table := core.EdgeMatchTable{MatchRecords: r.MatchRecords, MatchStages: r.MatchStages, Matches: r.Matches}
	if err := table.Validate(); err != nil {
		t.Fatalf("the match table does not validate: %v", err)
	}
	matches := make([]edgeMatch, 0, len(r.Matches))
	for index := range r.Matches {
		match, err := table.Match(index)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(match)
		if err != nil {
			t.Fatal(err)
		}
		var decoded edgeMatch
		decodeJSON(t, strings.NewReader(string(encoded)), &decoded)
		matches = append(matches, decoded)
	}
	return matches
}

// edgeMatch は関連付けの結果 1 件の項目を test 側で固定する。
type edgeMatch struct {
	OriginRef               core.RecordLocator     `json:"originRef"`
	CandidateRef            core.RecordLocator     `json:"candidateRef"`
	StageKey                string                 `json:"stageKey"`
	Conditions              []core.MatchCondition  `json:"conditions"`
	Assumptions             []core.MatchAssumption `json:"assumptions"`
	TimeWindow              core.TimeWindow        `json:"timeWindow"`
	TimeComparison          core.TimeComparison    `json:"timeComparison"`
	ClockDependencyNote     string                 `json:"clockDependencyNote"`
	StageTallies            []matchStageTally      `json:"stageTallies"`
	IndistinguishableGroups [][]core.RecordLocator `json:"indistinguishableGroups"`
	UnresolvedReasons       []string               `json:"unresolvedReasons"`
}

// matchStageTally は 1 つの起点に対して段階 1 つが挙げた候補の数え方である。
type matchStageTally struct {
	StageKey             string `json:"stageKey"`
	MemberCount          int64  `json:"memberCount"`
	DistinctProcessCount *int64 `json:"distinctProcessCount,omitempty"`
	EmptyReason          string `json:"emptyReason,omitempty"`
}

// candidateEdgeManifest は graph-manifest.json の候補のエッジの期待値である。
type candidateEdgeManifest struct {
	EdgeKind string `json:"edgeKind"`
	// SourceNodeKind は、同じ種別の候補のエッジのうち期待値を持つ 1 本を、起点のノードの
	// 種別で指定する。
	SourceNodeKind string `json:"sourceNodeKind"`
	MatchCount     int64  `json:"matchCount"`
	EvidenceCount  int64  `json:"evidenceCount"`
	StageTallies   []struct {
		StageKey             string `json:"stageKey"`
		MemberCount          int64  `json:"memberCount"`
		DistinctProcessCount *int64 `json:"distinctProcessCount,omitempty"`
	} `json:"stageTallies"`
	IndistinguishableGroupCount int `json:"indistinguishableGroupCount"`
	IndistinguishableGroupSize  int `json:"indistinguishableGroupSize"`
	Matches                     []struct {
		OriginFileName          string   `json:"originFileName"`
		OriginLineNumber        int64    `json:"originLineNumber"`
		CandidateFileName       string   `json:"candidateFileName"`
		CandidateSequenceNumber int64    `json:"candidateSequenceNumber"`
		StageKey                string   `json:"stageKey"`
		UsedConditionKeys       []string `json:"usedConditionKeys"`
		WindowKind              string   `json:"windowKind"`
		ComparisonUnit          string   `json:"comparisonUnit"`
		AssumptionKeys          []string `json:"assumptionKeys"`
	} `json:"matches"`
}

func candidateEdgeFixture(t *testing.T) candidateEdgeManifest {
	t.Helper()
	data, err := os.ReadFile(runFixtureDir + "graph-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		CandidateEdge candidateEdgeManifest `json:"candidateEdge"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest.CandidateEdge
}

func requestEdge(t *testing.T, handler http.Handler, id, query string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		withAllMatchConditions(edgesPath+id+"?"+query), nil))
	return response
}

func decodeEdge(t *testing.T, handler http.Handler, id, query string) edgeResponse {
	t.Helper()
	response := requestEdge(t, handler, id, query)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded edgeResponse
	decodeJSON(t, response.Body, &decoded)
	return decoded
}

// graphEdgeOfKind は`/api/v0/graph` の応答から、種別で指定したエッジを走査の順に 1 本読む。
func graphEdgeOfKind(t *testing.T, handler http.Handler, kind string) graphEdge {
	t.Helper()
	for _, edge := range decodeGraph(t, handler, wholeGraphQuery).Edges {
		if edge.Kind == kind {
			return edge
		}
	}
	t.Fatalf("the subgraph carries no edge of the kind %q", kind)
	return graphEdge{}
}

// fixtureCandidateEdge は`/api/v0/graph` の応答から、manifest の候補のエッジを種別と起点のノードの
// 種別で指定して読む。
func fixtureCandidateEdge(
	t *testing.T, handler http.Handler, want candidateEdgeManifest,
) graphEdge {
	t.Helper()
	return fixtureCandidateEdgeOf(t, decodeGraph(t, handler, wholeGraphQuery), want)
}

// fixtureCandidateEdgeOf は読んだ`/api/v0/graph` の応答から、manifest の候補のエッジを種別と起点の
// ノードの種別で指定して読む。
func fixtureCandidateEdgeOf(
	t *testing.T, subgraph graphResponse, want candidateEdgeManifest,
) graphEdge {
	t.Helper()
	nodeKinds := make(map[string]string, len(subgraph.Nodes))
	for _, node := range subgraph.Nodes {
		nodeKinds[node.Id] = node.Kind
	}
	for _, edge := range subgraph.Edges {
		if edge.Kind == want.EdgeKind && nodeKinds[edge.SourceNodeId] == want.SourceNodeKind {
			return edge
		}
	}
	t.Fatalf("the subgraph carries no %q edge from a %q node", want.EdgeKind, want.SourceNodeKind)
	return graphEdge{}
}

// anyGraphEdgeId は要求の形だけを確かめる test のために、エッジの識別子を 1 つ返す。
func anyGraphEdgeId(t *testing.T, handler http.Handler) string {
	t.Helper()
	edges := decodeGraph(t, handler, wholeGraphQuery).Edges
	if len(edges) == 0 {
		t.Fatal("the subgraph carries no edge")
	}
	return edges[0].Id
}

func TestEdgeEndpointReturnsTheConditionsOfTheCandidateEdge(t *testing.T) {
	want := candidateEdgeFixture(t)
	handler := graphHandler(t)
	edge := fixtureCandidateEdge(t, handler, want)
	if edge.State != "candidate" {
		t.Fatalf("the %s edge carries the state %q, want candidate", edge.Kind, edge.State)
	}
	detail := decodeEdge(t, handler, edge.Id, "")
	if detail.Edge.Id != edge.Id || detail.Edge.EvidenceCount != edge.EvidenceCount {
		t.Fatalf("the detail returned the edge %q with %d evidence records, want %q and %d",
			detail.Edge.Id, detail.Edge.EvidenceCount, edge.Id, edge.EvidenceCount)
	}
	if detail.SourceNode.Id != edge.SourceNodeId || detail.TargetNode.Id != edge.TargetNodeId {
		t.Fatalf("the detail returned the nodes %q and %q, want %q and %q",
			detail.SourceNode.Id, detail.TargetNode.Id, edge.SourceNodeId, edge.TargetNodeId)
	}
	// **2 件以上の関連付けを 1 件にまとめない。** 同じ起点が con と est の 2 件を候補に挙げる。
	if detail.MatchCount != want.MatchCount || int64(len(detail.Matches)) != want.MatchCount {
		t.Fatalf("the detail carries %d matches, want %d", detail.MatchCount, want.MatchCount)
	}
	// **根拠は起点 1 件と候補 2 件である。** 起点のレコードを候補ごとに入れ直さない。
	if detail.Edge.EvidenceCount != want.EvidenceCount {
		t.Fatalf("the candidate edge carries %d evidence records, want %d",
			detail.Edge.EvidenceCount, want.EvidenceCount)
	}
	matches := detail.expandedMatches(t)
	// **同じ起点の関連付けは、段階を 1 つだけ置いて共有する。** 段階の値を関連付けごとに繰り返さない。
	stageOfOrigin := make(map[string]int)
	for index, match := range matches {
		stage, placed := stageOfOrigin[match.OriginRef.RecordRawTextRef]
		if placed && stage != detail.Matches[index].Stage {
			t.Errorf("match %d points at the stage %d, the same origin is at the stage %d", index,
				detail.Matches[index].Stage, stage)
		}
		stageOfOrigin[match.OriginRef.RecordRawTextRef] = detail.Matches[index].Stage
	}
	if len(detail.MatchStages) != len(stageOfOrigin) {
		t.Errorf("the response places %d stages for %d origins", len(detail.MatchStages), len(stageOfOrigin))
	}
	for index, wantMatch := range want.Matches {
		match := matches[index]
		if match.StageKey != wantMatch.StageKey {
			t.Errorf("match %d comes from the stage %q, want %q",
				index, match.StageKey, wantMatch.StageKey)
		}
		if match.OriginRef.SourceFileName != wantMatch.OriginFileName ||
			match.OriginRef.LineNumber == nil ||
			*match.OriginRef.LineNumber != wantMatch.OriginLineNumber {
			t.Errorf("match %d starts at %+v, want the line %d of %q", index,
				match.OriginRef, wantMatch.OriginLineNumber, wantMatch.OriginFileName)
		}
		if match.CandidateRef.SourceFileName != wantMatch.CandidateFileName ||
			match.CandidateRef.SequenceNumber == nil ||
			*match.CandidateRef.SequenceNumber != wantMatch.CandidateSequenceNumber {
			t.Errorf("match %d points at %+v, want the sequence number %d of %q", index,
				match.CandidateRef, wantMatch.CandidateSequenceNumber, wantMatch.CandidateFileName)
		}
		assertUsedConditions(t, match.Conditions, wantMatch.UsedConditionKeys)
		if string(match.TimeWindow.WindowKind) != wantMatch.WindowKind {
			t.Errorf("match %d used the window %q, want %q",
				index, match.TimeWindow.WindowKind, wantMatch.WindowKind)
		}
		if string(match.TimeComparison.ComparisonUnit) != wantMatch.ComparisonUnit {
			t.Errorf("match %d compared time in %q, want %q",
				index, match.TimeComparison.ComparisonUnit, wantMatch.ComparisonUnit)
		}
		assertAssumptionKeys(t, match.Assumptions, wantMatch.AssumptionKeys)
		if len(match.UnresolvedReasons) == 0 {
			t.Errorf("match %d carries no unresolved reason, want at least one", index)
		}
		// **前提と別の項目である。** 段階 1 から引き継ぐ割当の条件が時計に依拠する箇所を持つ。
		if match.ClockDependencyNote == "" {
			t.Errorf("match %d carries no clock dependency note, want one", index)
		}
		assertStageItems(t, index, match, want)
	}
}

// assertStageItems は候補の確度を読む項目を確かめる。
func assertStageItems(t *testing.T, index int, match edgeMatch, want candidateEdgeManifest) {
	t.Helper()
	if len(match.StageTallies) != len(want.StageTallies) {
		t.Fatalf("match %d carries the stages %+v, want %+v",
			index, match.StageTallies, want.StageTallies)
	}
	for position, wantTally := range want.StageTallies {
		tally := match.StageTallies[position]
		if tally.StageKey != wantTally.StageKey {
			t.Errorf("match %d carries the stage %q at the position %d, want %q",
				index, tally.StageKey, position, wantTally.StageKey)
		}
		if tally.MemberCount != wantTally.MemberCount {
			t.Errorf("match %d reports %d candidates in the stage %q, want %d",
				index, tally.MemberCount, wantTally.StageKey, wantTally.MemberCount)
		}
		if !equalInt64Pointers(tally.DistinctProcessCount, wantTally.DistinctProcessCount) {
			t.Errorf("match %d reports %+v distinct processes in the stage %q, want %+v",
				index, tally.DistinctProcessCount, wantTally.StageKey,
				wantTally.DistinctProcessCount)
		}
		if tally.EmptyReason != "" {
			t.Errorf("match %d reports the empty reason %q in the stage %q, want none",
				index, tally.EmptyReason, wantTally.StageKey)
		}
	}
	// 末尾の段階がこの関連付けを出した段階であり、その段階は候補を 1 件以上持つ。
	last := match.StageTallies[len(match.StageTallies)-1]
	if last.StageKey != match.StageKey {
		t.Errorf("match %d ends at the stage %q while the match comes from %q",
			index, last.StageKey, match.StageKey)
	}
	if last.MemberCount < 1 {
		t.Errorf("match %d counts %d candidates in its own stage, want at least 1",
			index, last.MemberCount)
	}
	// 段階 1 は時刻を比べずに絞る。段階 2 はそこから減らすだけであり、増やさない。
	if first := match.StageTallies[0]; first.MemberCount < last.MemberCount {
		t.Errorf("match %d counts %d candidates in the stage %q and %d in the stage %q, "+
			"want the later stage to carry no more than the earlier one",
			index, first.MemberCount, first.StageKey, last.MemberCount, last.StageKey)
	}
	if len(match.IndistinguishableGroups) != want.IndistinguishableGroupCount {
		t.Fatalf("match %d carries %d indistinguishable groups, want %d",
			index, len(match.IndistinguishableGroups), want.IndistinguishableGroupCount)
	}
	if len(match.IndistinguishableGroups[0]) != want.IndistinguishableGroupSize {
		t.Errorf("match %d carries a group of %d, want %d",
			index, len(match.IndistinguishableGroups[0]), want.IndistinguishableGroupSize)
	}
}

// equalInt64Pointers は、出る値と出ない値を分けて 2 つの数を比べる。
func equalInt64Pointers(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// assertUsedConditions は use が used の条件の種別を、応答の並び順で確かめる。
func assertUsedConditions(t *testing.T, conditions []core.MatchCondition, want []string) {
	t.Helper()
	used := make([]string, 0, len(conditions))
	for _, condition := range conditions {
		if condition.Use == core.ConditionUseUsed {
			used = append(used, string(condition.ConditionKey))
		}
	}
	if strings.Join(used, ",") != strings.Join(want, ",") {
		t.Errorf("the match used the conditions %v, want %v", used, want)
	}
}

func assertAssumptionKeys(t *testing.T, assumptions []core.MatchAssumption, want []string) {
	t.Helper()
	keys := make([]string, 0, len(assumptions))
	for _, assumption := range assumptions {
		keys = append(keys, string(assumption.AssumptionKey))
	}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Errorf("the match rests on the assumptions %v, want %v", keys, want)
	}
}

// 観測から直接作ったエッジは関連付けを持たない。
func TestEdgeEndpointReturnsNoMatchOnAnObservedEdge(t *testing.T) {
	handler := graphHandler(t)
	edge := graphEdgeOfKind(t, handler, "process_parent_child")
	if edge.State != "observed" {
		t.Fatalf("the %s edge carries the state %q, want observed", edge.Kind, edge.State)
	}
	response := requestEdge(t, handler, edge.Id, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	// **要素数 0 の集合を null で返さない。** 生の body で確かめる。
	for _, empty := range []string{
		`"matchRecords":[]`, `"matchStages":[]`, `"matches":[]`, `"authentications":[]`,
	} {
		if !strings.Contains(response.Body.String(), empty) {
			t.Errorf("body=%s want it to contain %s", response.Body.String(), empty)
		}
	}
	// レコードの組を持たないエッジは、組の項目を持たない。
	if strings.Contains(response.Body.String(), `"recordPair`) {
		t.Errorf("body=%s want no record pair of an observed edge", response.Body.String())
	}
	var detail edgeResponse
	decodeJSON(t, response.Body, &detail)
	if detail.MatchCount != 0 || len(detail.Matches) != 0 {
		t.Fatalf("the observed edge carries %d matches, want 0", detail.MatchCount)
	}
	// 2 つの軸を別に返す。参照だけの親は生成のレコードも持たない。
	if detail.SourceNode.Observation != string(core.NodeObservationReferenced) ||
		detail.SourceNode.CreationRecord != string(core.NodeCreationRecordAbsent) {
		t.Errorf("the parent node carries %q/%q, want referenced/absent",
			detail.SourceNode.Observation, detail.SourceNode.CreationRecord)
	}
	if detail.TargetNode.Observation != string(core.NodeObservationObserved) ||
		detail.TargetNode.CreationRecord != string(core.NodeCreationRecordPresent) {
		t.Errorf("the child node carries %q/%q, want observed/present",
			detail.TargetNode.Observation, detail.TargetNode.CreationRecord)
	}
}

func TestEdgeEndpointReturnsTheDirectedFileCopyEdge(t *testing.T) {
	handler := graphHandler(t)
	edge := graphEdgeOfKind(t, handler, "file_copy")
	if edge.State != "observed" {
		t.Fatalf("the %s edge carries the state %q, want observed", edge.Kind, edge.State)
	}
	detail := decodeEdge(t, handler, edge.Id, "")
	if detail.SourceNode.Kind != "file" || detail.TargetNode.Kind != "file" {
		t.Fatalf("the copy edge connects %q to %q, want file to file",
			detail.SourceNode.Kind, detail.TargetNode.Kind)
	}
	if detail.SourceNode.Id != edge.SourceNodeId || detail.TargetNode.Id != edge.TargetNodeId {
		t.Fatalf("the detail returned the nodes %q and %q, want %q and %q",
			detail.SourceNode.Id, detail.TargetNode.Id, edge.SourceNodeId, edge.TargetNodeId)
	}
	if detail.Edge.EvidenceCount != 1 || len(detail.Edge.Evidence) != 1 {
		t.Fatalf("the copy edge carries %d evidence records and %d returned, want 1 and 1",
			detail.Edge.EvidenceCount, len(detail.Edge.Evidence))
	}
	if detail.MatchCount != 0 || len(detail.Matches) != 0 {
		t.Fatalf("the observed copy edge carries %d matches, want 0", detail.MatchCount)
	}
}

func TestEdgeEndpointReturnsTheFileContentMatchCandidate(t *testing.T) {
	result := importResultOfFiles(t, pipeline.SourcePlan{
		FormatKey: markIIFormatKey,
		FileName:  "graph-file-match.log", OriginPath: "testdata/graph-file-match.log",
	})
	handler := testHandler(result)
	edge := graphEdgeOfKind(t, handler, "file_content_match")
	if edge.State != "candidate" {
		t.Fatalf("the %s edge carries the state %q, want candidate", edge.Kind, edge.State)
	}
	detail := decodeEdge(t, handler, edge.Id, "")
	if detail.SourceNode.Kind != "file" || detail.TargetNode.Kind != "file" {
		t.Fatalf("the content match connects %q to %q, want file to file",
			detail.SourceNode.Kind, detail.TargetNode.Kind)
	}
	if detail.Edge.EvidenceCount != 2 || len(detail.Edge.Evidence) != 2 {
		t.Fatalf("the content match carries %d evidence records and %d returned, want 2 and 2",
			detail.Edge.EvidenceCount, len(detail.Edge.Evidence))
	}
	if detail.MatchCount != 0 || len(detail.Matches) != 0 {
		t.Fatalf("the content match carries %d matches, want 0", detail.MatchCount)
	}
}

// 根拠と関連付けを全件返す。1 回の要求で、数えた件数と並べた要素の数が一致する。
func TestEdgeEndpointReturnsEveryEvidenceRecordAndEveryMatch(t *testing.T) {
	want := candidateEdgeFixture(t)
	handler := graphHandler(t)
	edge := fixtureCandidateEdge(t, handler, want)
	detail := decodeEdge(t, handler, edge.Id, "")
	if detail.Edge.EvidenceCount != int64(len(detail.Edge.Evidence)) {
		t.Errorf("evidenceCount=%d with %d evidence records, want the same number",
			detail.Edge.EvidenceCount, len(detail.Edge.Evidence))
	}
	if detail.Edge.EvidenceCount != want.EvidenceCount {
		t.Errorf("evidenceCount=%d want %d", detail.Edge.EvidenceCount, want.EvidenceCount)
	}
	if detail.MatchCount != int64(len(detail.Matches)) {
		t.Errorf("matchCount=%d with %d matches, want the same number",
			detail.MatchCount, len(detail.Matches))
	}
	if detail.MatchCount != want.MatchCount {
		t.Errorf("matchCount=%d want %d", detail.MatchCount, want.MatchCount)
	}
	// 根拠のレコードは互いに別の位置を指す。
	seen := make(map[string]bool, len(detail.Edge.Evidence))
	for _, evidence := range detail.Edge.Evidence {
		ref := evidence.RecordRef.RecordRawTextRef
		if seen[ref] {
			t.Errorf("the detail returned the record %q twice", ref)
		}
		seen[ref] = true
	}
}

// **上限と続きを取る位置を受けない。** 綴りを通知せずに既定値で処理しないよう、未知の
// 項目として退ける。
func TestEdgeEndpointRejectsTheLimitAndTheCursor(t *testing.T) {
	handler := graphHandler(t)
	id := anyGraphEdgeId(t, handler)
	for _, testCase := range []struct {
		name  string
		query string
	}{
		{"evidence limit", "evidenceLimit=20"},
		{"match limit", "matchLimit=20"},
		{"limit", "limit=20"},
		{"cursor", "cursor=any-cursor"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := requestEdge(t, handler, id, testCase.query)
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

func TestEdgeEndpointRejectsAnUnknownEdgeId(t *testing.T) {
	response := requestEdge(t, graphHandler(t), "e:ran_on:absent", "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d want %d body=%s", response.Code, http.StatusNotFound,
			response.Body.String())
	}
	var apiError core.ApiError
	decodeJSON(t, response.Body, &apiError)
	if apiError.Code != core.ApiErrorCodeRecordNotFound {
		t.Errorf("code=%q want record_not_found", apiError.Code)
	}
}

func TestEdgeEndpointRejectsInvalidRequests(t *testing.T) {
	handler := graphHandler(t)
	id := anyGraphEdgeId(t, handler)
	for _, testCase := range []struct {
		name        string
		query       string
		wantMissing []string
	}{
		{"an unsupported parameter", "depth=1", nil},
		{"a repeated event category", "eventCategory=file&eventCategory=process", nil},
		{"an empty event category", "eventCategory=", nil},
		{"an empty event action", "eventAction=", nil},
		{"an empty destination port", "destinationPort=", nil},
		{"an empty destination port absence", "destinationPortAbsent=", nil},
		{"an empty logon type", "logonType=", nil},
		{"an empty logon type absence", "logonTypeAbsent=", nil},
		{"a repeated logon type", "logonType=3&logonType=10", nil},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := requestEdge(t, handler, id, testCase.query)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want %d body=%s", response.Code, http.StatusBadRequest,
					response.Body.String())
			}
			var apiError core.ApiError
			decodeJSON(t, response.Body, &apiError)
			if apiError.Code != core.ApiErrorCodeInvalidRequest {
				t.Fatalf("code=%q want invalid_request", apiError.Code)
			}
			if strings.Join(apiError.MissingParameters, ",") !=
				strings.Join(testCase.wantMissing, ",") {
				t.Errorf("missingParameters=%v want %v",
					apiError.MissingParameters, testCase.wantMissing)
			}
		})
	}
}

// 応答は根拠の区分を持ち、区分を指した要求はその区分の根拠だけを返す。
func TestEdgeEndpointNarrowsTheEvidenceToTheRequestedGroup(t *testing.T) {
	handler := graphHandler(t)
	id := fixtureCandidateEdge(t, handler, candidateEdgeFixture(t)).Id
	whole := decodeEdge(t, handler, id, "")
	if len(whole.EvidenceGroups) == 0 {
		t.Fatal("the detail carries no evidence group, so the check reads nothing")
	}
	var summed int64
	for _, group := range whole.EvidenceGroups {
		if err := group.Validate(); err != nil {
			t.Errorf("an evidence group = %v, want a valid group", err)
		}
		summed += group.EvidenceCount
	}
	if summed != whole.Edge.EvidenceCount {
		t.Errorf("the groups carry %d evidence records in total, want the edge count %d",
			summed, whole.Edge.EvidenceCount)
	}
	group := selectableGroup(t, whole.EvidenceGroups)
	narrowed := decodeEdge(t, handler, id, evidenceGroupQuery(*group.Selector))
	if narrowed.Edge.EvidenceCount != group.EvidenceCount {
		t.Errorf("the narrowed detail carries %d evidence records, want the group count %d",
			narrowed.Edge.EvidenceCount, group.EvidenceCount)
	}
	if int64(len(narrowed.Edge.Evidence)) != narrowed.Edge.EvidenceCount {
		t.Errorf("the narrowed detail returned %d evidence records, want the count %d",
			len(narrowed.Edge.Evidence), narrowed.Edge.EvidenceCount)
	}
	// **区分の一覧は絞り込みで変わらない。**
	if len(narrowed.EvidenceGroups) != len(whole.EvidenceGroups) {
		t.Errorf("the narrowed detail carries %d groups, want the whole count %d",
			len(narrowed.EvidenceGroups), len(whole.EvidenceGroups))
	}
}

// selectableGroup は要求で指せる区分を 1 つ返す。
//
// **指せる区分は応答が selector を持つ。** 画面が指す値を組み立てる必要がなく、
// 応答が返した値をそのまま要求へ返す。
func selectableGroup(t *testing.T, groups []core.EdgeEvidenceGroup) core.EdgeEvidenceGroup {
	t.Helper()
	for _, group := range groups {
		if group.Selector != nil {
			return group
		}
	}
	t.Fatal("no evidence group carries a selector, so no group can be named by a request")
	return core.EdgeEvidenceGroup{}
}

// evidenceGroupQuery は、応答が返した区分を指す値を要求の項目へ直す。
// 観測の種別を持たない HTTP の要求の区分は、観測の種別の項目を載せない。
func evidenceGroupQuery(selector core.EdgeEvidenceSelector) string {
	var items []string
	if selector.EventCategory != "" {
		items = append(items, "eventCategory="+url.QueryEscape(selector.EventCategory),
			"eventAction="+url.QueryEscape(selector.EventAction))
	}
	switch {
	case selector.HttpStatusAbsent:
		items = append(items, "httpStatusAbsent=true")
	case selector.HttpStatus != "":
		items = append(items, "httpStatus="+url.QueryEscape(selector.HttpStatus))
	}
	if selector.DestinationPortAbsent {
		items = append(items, "destinationPortAbsent=true")
	} else {
		items = append(items, "destinationPort="+url.QueryEscape(selector.DestinationPort))
	}
	if selector.LogonTypeAbsent {
		items = append(items, "logonTypeAbsent=true")
	} else {
		items = append(items, "logonType="+url.QueryEscape(selector.LogonType))
	}
	return strings.Join(items, "&")
}

// HTTP の要求の区分は HTTP の状態ごとに分かれ、観測の種別を持たない区分も状態で指せる。
func TestEdgeEndpointNarrowsTheHttpRequestsByTheStatus(t *testing.T) {
	handler := graphHandler(t)
	page := decodeGraph(t, handler, wholeGraphQuery)
	named := 0
	for _, edge := range page.Edges {
		whole := decodeEdge(t, handler, edge.Id, "")
		for _, group := range whole.EvidenceGroups {
			if group.Selector == nil || group.Selector.HttpStatus == "" {
				continue
			}
			named++
			narrowed := decodeEdge(t, handler, edge.Id, evidenceGroupQuery(*group.Selector))
			if narrowed.Edge.EvidenceCount != group.EvidenceCount {
				t.Errorf("the status %s narrowed %d records, want %d",
					group.Selector.HttpStatus, narrowed.Edge.EvidenceCount, group.EvidenceCount)
			}
		}
	}
	if named == 0 {
		t.Fatal("no HTTP request group carries a status to name")
	}
}

// 接続先 port とログオンの種別のそれぞれで、値と、欄の不在を指す条件は排他である。
func TestEdgeEndpointRejectsTheContradictingValueAndAbsenceFilter(t *testing.T) {
	handler := graphHandler(t)
	id := anyGraphEdgeId(t, handler)

	for _, pair := range []struct{ value, absent string }{
		{"destinationPort=5985", "destinationPortAbsent"},
		{"logonType=3", "logonTypeAbsent"},
		{"httpStatus=404", "httpStatusAbsent"},
	} {
		response := requestEdge(t, handler, id, pair.value+"&"+pair.absent+"=true")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want %d body=%s", response.Code, http.StatusBadRequest,
				response.Body.String())
		}
		// 反対側。片方だけを与えた要求は通る。
		for _, query := range []string{pair.value, pair.absent + "=true"} {
			if accepted := requestEdge(t, handler, id, query); accepted.Code != http.StatusOK {
				t.Errorf("status=%d want %d for %q body=%s",
					accepted.Code, http.StatusOK, query, accepted.Body.String())
			}
		}
		// true 以外の文字列を退ける。
		if other := requestEdge(t, handler, id,
			pair.absent+"=false"); other.Code != http.StatusBadRequest {
			t.Errorf("status=%d want %d for %s outside true",
				other.Code, http.StatusBadRequest, pair.absent)
		}
	}
}

func TestEdgeEndpointRejectsOtherMethods(t *testing.T) {
	handler := graphHandler(t)
	id := anyGraphEdgeId(t, handler)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response,
		httptest.NewRequest(http.MethodDelete,
			withAllMatchConditions(edgesPath+id), nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Errorf("status=%d want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

// 遠隔のセッションの候補のエッジは、成立の根拠を応答で持つ。
//
// **応答の項目の増減を decode が検出する。** edgeResponse は DisallowUnknownFields で
// 読むため、handler が項目を足すとこの test が失敗する。
func TestEdgeEndpointCarriesTheAssignmentBasis(t *testing.T) {
	handler := testHandler(importResultOfFiles(t, pipeline.SourcePlan{
		OriginPath: "testdata/graph-markii-session.log",
		FileName:   "graph-markii-session.log",
		FormatKey:  markIIFormatKey,
	}))
	subgraph := decodeGraph(t, handler,
		"depth=1&edgeKind=terminal_remote_session")
	if len(subgraph.Edges) != 1 {
		t.Fatalf("the subgraph carries %d remote session relations, want 1",
			len(subgraph.Edges))
	}
	detail := decodeEdge(t, handler, subgraph.Edges[0].Id, "")
	if len(detail.AssignmentBases) != 1 {
		t.Fatalf("the response carries %d assignment bases, want 1",
			len(detail.AssignmentBases))
	}
	basis := detail.AssignmentBases[0]
	if basis.ClientIp != "192.0.2.1" {
		t.Errorf("the basis used the address %q, want 192.0.2.1", basis.ClientIp)
	}
	if err := basis.Validate(); err != nil {
		t.Errorf("the response carries an invalid assignment basis: %v", err)
	}
	if len(detail.Matches) != 0 || detail.MatchCount != 0 {
		t.Errorf("the relation carries %d matches, want 0", detail.MatchCount)
	}
}

// 収集元に付けた割当の IP から作った terminal_address のエッジは、その割当を応答で持つ。
func TestEdgeEndpointCarriesTheTerminalAssignmentOfTheAssignedAddress(t *testing.T) {
	// Squid のレコードは自身の端末の欄を持たないため、割当の端末に置かれる。
	handler := testHandler(importResultOfFiles(t, pipeline.SourcePlan{
		OriginPath: "testdata/graph-squid.log", FileName: "graph-squid.log", FormatKey: squidFormatKey,
	}))
	body := analystAssignmentMap(t, handler)
	body["appliesToSourceId"] = body["sourceId"]
	requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath, encodeBody(t, body), http.StatusCreated)

	subgraph := decodeGraph(t, handler, "depth=1&edgeKind=terminal_address")
	ipNodes := make(map[string]string, len(subgraph.Nodes))
	for _, node := range subgraph.Nodes {
		if node.Kind == string(core.NodeKindIp) && len(node.Identity) == 1 {
			ipNodes[node.Id] = node.Identity[0].Value
		}
	}
	edgeId := ""
	for _, edge := range subgraph.Edges {
		if ipNodes[edge.TargetNodeId] == body["clientIp"] {
			edgeId = edge.Id
		}
	}
	if edgeId == "" {
		t.Fatalf("the subgraph carries no terminal_address edge to %v", body["clientIp"])
	}
	detail := decodeEdge(t, handler, edgeId, "")
	if len(detail.TerminalAssignments) != 1 ||
		detail.TerminalAssignments[0].Origin != core.TerminalAssignmentOriginAnalystSupplied {
		t.Errorf("the response carries the assignments %+v, want the analyst assignment",
			detail.TerminalAssignments)
	}
	// Squid のレコードは端末の IP を記録しない。
	if detail.ObservedInRecords == nil || *detail.ObservedInRecords {
		t.Errorf("observedInRecords = %v, want false beside the assignments", detail.ObservedInRecords)
	}
}

// 観測から直接作ったエッジは成立の根拠を持たない。
func TestEdgeEndpointCarriesNoAssignmentBasisOnAnObservedEdge(t *testing.T) {
	handler := graphHandler(t)
	subgraph := decodeGraph(t, handler,
		"depth=1&edgeKind=ran_on")
	if len(subgraph.Edges) == 0 {
		t.Fatal("the subgraph carries no observed relation")
	}
	detail := decodeEdge(t, handler, subgraph.Edges[0].Id, "")
	if len(detail.AssignmentBases) != 0 {
		t.Errorf("the observed relation carries %d assignment bases, want 0",
			len(detail.AssignmentBases))
	}
}
