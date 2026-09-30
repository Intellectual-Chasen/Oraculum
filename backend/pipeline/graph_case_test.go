// in-package test: 非公開のグラフのエッジと根拠の位置を読み、案件ごとに関係を導くことを確かめる。
package pipeline

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// auditd の収集元。
const (
	// caseParentSource は番号 1000 のプロセスが起動した記録である。
	caseParentSource = "type=SYSCALL msg=audit(1000000000.100:1): ppid=900 pid=1000 comm=\"bash\" exe=\"/usr/bin/bash\"\n" +
		"type=EXECVE msg=audit(1000000000.100:1): argc=1 a0=\"bash\"\n"
	// caseChildSource は親の番号 1000 を持つプロセスが、親より後に起動した記録である。
	caseChildSource = "type=SYSCALL msg=audit(1000000900.200:2): ppid=1000 pid=2000 comm=\"child\" exe=\"/usr/bin/child\"\n" +
		"type=EXECVE msg=audit(1000000900.200:2): argc=1 a0=\"child\"\n"
)

// markii 形式の収集元。同じ端末の別のファイルが同じ sha256 を持つ。
const (
	caseFileSourceLeft = `02/01/2000 13:00:00.000 +0900 sn=1 evt=file subEvt=close psGUID={P1} tmid=T1 com="PC01" ` +
		`path="C:\source.zip" sha256=1e5ac0d1c4f0dbb7b3c1a2e5d6f708192a3b4c5d6e7f80912a3b4c5d6e7f8091` + "\n"
	caseFileSourceRight = `02/01/2000 13:00:01.000 +0900 sn=2 evt=file subEvt=close psGUID={P1} tmid=T1 com="PC01" ` +
		`path="E:\destination.zip" sha256=1e5ac0d1c4f0dbb7b3c1a2e5d6f708192a3b4c5d6e7f80912a3b4c5d6e7f8091` + "\n"
)

// caseSource は取り込む収集元 1 件と、付ける案件である。
type caseSource struct {
	name, format, content string
	caseId                *string
}

func caseOf(value string) *string { return &value }

// caseImport は収集元を取り込みの実行器に通す。
func caseImport(t *testing.T, sources ...caseSource) ImportResult {
	t.Helper()
	contents := make(map[string]string, len(sources))
	plans := make([]SourcePlan, len(sources))
	for i, source := range sources {
		contents[source.name] = source.content
		plans[i] = SourcePlan{OriginPath: source.name, FileName: source.name,
			FormatKey: core.FormatKey(source.format), CaseId: source.caseId}
	}
	runner, err := NewRunner(Config{
		Open: func(originPath string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(contents[originPath])), nil
		},
		Parsers: NewTestFormatRegistry(), Minter: DigestMinter{}, Ordinals: NewInMemoryOrdinals(),
		Sanitize: func(value string) string { return value },
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(plans)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// caseLineageGraph は auditd の収集元のそれぞれに、分析者が同じ端末を与えたグラフを組む。
func caseLineageGraph(t *testing.T, sources ...caseSource) Graph {
	t.Helper()
	result := caseImport(t, sources...)
	assignments := make([]core.TerminalAssignment, 0, len(result.publications))
	for i := range result.publications {
		single := result
		single.publications = result.publications[i : i+1]
		assignments = append(assignments, analystAssignmentFor(t, single))
	}
	return NewGraph(result.WithAnalystTerminalAssignments(assignments), AllMatchConditions())
}

func auditdSource(name, content string, caseId *string) caseSource {
	return caseSource{name: name, format: string(AuditdFormatKeyText), content: content, caseId: caseId}
}

func markIISource(name, content string, caseId *string) caseSource {
	return caseSource{name: name, format: string(MarkIIFormatKey), content: content, caseId: caseId}
}

// edgesOfKind はその種別のエッジを返す。
func edgesOfKind(graph Graph, kind core.EdgeKind) []graphEdge {
	var edges []graphEdge
	for _, edge := range graph.edges {
		if edge.kind == kind {
			edges = append(edges, edge)
		}
	}
	return edges
}

// 親と子を別の案件で取り込むと、親子の候補を導かない。同じ案件なら導く。
func TestGraphDerivesTheParentOnlyWithinACase(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		parentCase  *string
		childCase   *string
		wantParents int
	}{
		{"no case", nil, nil, 1},
		{"one case", caseOf("challenge"), caseOf("challenge"), 1},
		{"two cases", caseOf("baseline"), caseOf("challenge"), 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			graph := caseLineageGraph(t,
				auditdSource("parent.log", caseParentSource, testCase.parentCase),
				auditdSource("child.log", caseChildSource, testCase.childCase))
			if got := len(processNodesOfPid(t, graph, "1000")); got != 1 {
				t.Fatalf("the parent pid reaches %d nodes, want 1", got)
			}
			if got := len(lineageParentChildEdges(graph)); got != testCase.wantParents {
				t.Errorf("the graph carries %d parent-child edges, want %d", got, testCase.wantParents)
			}
		})
	}
}

// 期間を読み取った収集元と端末を与える収集元が別の案件にある分析者の割当は、レコードへ
// 端末を与えない。同じ案件なら与える。
func TestGraphAppliesAnAnalystTerminalOnlyWithinACase(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		periodCase   *string
		appliedCase  *string
		wantTerminal bool
	}{
		{"no case", nil, nil, true},
		{"one case", caseOf("challenge"), caseOf("challenge"), true},
		{"two cases", caseOf("baseline"), caseOf("challenge"), false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result := caseImport(t,
				auditdSource("period.log", caseParentSource, testCase.periodCase),
				auditdSource("applied.log", caseChildSource, testCase.appliedCase))
			period, applied := result.publications[0], result.publications[1]
			single := result
			single.publications = result.publications[1:2]
			assignment := analystAssignmentFor(t, single)
			assignment.SourceId = period.status.SourceId
			assignment.SourceContentSha256 = period.status.Scope.SourceContentSha256
			graph := NewGraph(result.WithAnalystTerminalAssignments(
				[]core.TerminalAssignment{assignment}), AllMatchConditions())
			terminal, _ := core.TerminalNodeKey(lineageTerminalId)
			at, found := graph.nodeAt[nodeIdOf(terminal)]
			carries := false
			if found {
				for _, evidence := range graph.nodes[at].evidence {
					if graph.records[evidence].locator.SourceId == applied.status.SourceId {
						carries = true
					}
				}
			}
			if carries != testCase.wantTerminal {
				t.Errorf("the terminal holds the records of applied.log: %t, want %t", carries, testCase.wantTerminal)
			}
		})
	}
}

// 同じ hash を持つ 2 つのファイルを別の案件で観測すると、内容の一致の候補を導かない。
func TestGraphMatchesFileContentOnlyWithinACase(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		left, right *string
		wantMatches int
	}{
		{"no case", nil, nil, 1},
		{"one case", caseOf("challenge"), caseOf("challenge"), 1},
		{"two cases", caseOf("baseline"), caseOf("challenge"), 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result := caseImport(t,
				markIISource("left.log", caseFileSourceLeft, testCase.left),
				markIISource("right.log", caseFileSourceRight, testCase.right))
			graph := NewGraph(result, AllMatchConditions())
			matches := edgesOfKind(graph, core.EdgeKindFileContentMatch)
			if len(matches) != testCase.wantMatches {
				t.Fatalf("the graph carries %d content matches, want %d", len(matches), testCase.wantMatches)
			}
			for _, edge := range matches {
				if cases := graph.casesOf(edge.evidence); len(cases) != 1 {
					t.Errorf("the content match holds the evidence of the cases %v, want one case", cases)
				}
			}
		})
	}
}

// Proxy の要求と端末の通信を別の案件で取り込むと、収集元をまたいだ関連付けの候補を導かない。
// 端末から IP への割当も案件の中の収集元から作るので、Proxy の要求の端末も決まらない。
func TestGraphMatchesAcrossSourcesOnlyWithinACase(t *testing.T) {
	markII, err := os.ReadFile("../internal/testdata/run/graph-markii.log")
	if err != nil {
		t.Fatal(err)
	}
	squid, err := os.ReadFile("../internal/testdata/run/graph-squid.log")
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		name          string
		endpoint      *string
		proxy         *string
		wantCandidate bool
	}{
		{"no case", nil, nil, true},
		{"one case", caseOf("challenge"), caseOf("challenge"), true},
		{"two cases", caseOf("baseline"), caseOf("challenge"), false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result := caseImport(t,
				markIISource("endpoint.log", string(markII), testCase.endpoint),
				caseSource{name: "proxy.log", format: string(SquidFormatKey), content: string(squid),
					caseId: testCase.proxy})
			graph := NewGraph(result, AllMatchConditions())
			matches := edgesOfKind(graph, core.EdgeKindCrossSourceConnectionMatch)
			if (len(matches) > 0) != testCase.wantCandidate {
				t.Fatalf("the graph carries %d cross-source matches, want present=%t",
					len(matches), testCase.wantCandidate)
			}
			for _, edge := range matches {
				if cases := graph.casesOf(edge.evidence); len(cases) != 1 {
					t.Errorf("the match holds the evidence of the cases %v, want one case", cases)
				}
			}
		})
	}
}

// sharedCaseGraph は、同じ端末の同じプロセスを 2 つの案件が 1 件ずつ記録したグラフを組む。
func sharedCaseGraph(t *testing.T) Graph {
	t.Helper()
	return NewGraph(caseImport(t,
		markIISource("left.log", caseFileSourceLeft, caseOf("baseline")),
		markIISource("right.log", caseFileSourceRight, caseOf("challenge"))), AllMatchConditions())
}

// caseCounts は案件ごとの件数を、案件の識別子で探せる表にする。
func caseCounts(counts []core.CaseEvidenceCount) map[string]int64 {
	table := make(map[string]int64, len(counts))
	for _, count := range counts {
		table[count.CaseId] = count.EvidenceCount
	}
	return table
}

// 部分グラフのエッジの案件ごとの件数は、全体の件数に和が等しく、案件で絞った要求の件数と
// 一致する。
func TestGraphEdgeCountsEvidenceByCase(t *testing.T) {
	graph := sharedCaseGraph(t)
	whole := graph.Query(GraphQuery{Depth: 1})
	byCase := map[string]map[string]int64{}
	spanning := 0
	for _, edge := range whole.Edges {
		if err := edge.Validate(); err != nil {
			t.Fatalf("edge %s: %v", edge.Id, err)
		}
		byCase[edge.Id] = caseCounts(edge.EvidenceByCase)
		if len(edge.EvidenceByCase) == 2 {
			spanning++
		}
	}
	if spanning == 0 {
		t.Fatal("no edge holds the evidence of both cases")
	}
	for _, caseId := range []string{"baseline", "challenge"} {
		query := GraphQuery{Depth: 1, RecordFilter: RecordFilter{Case: caseId}}
		query.Validate()
		for _, edge := range graph.Query(query).Edges {
			if edge.EvidenceCount != byCase[edge.Id][caseId] {
				t.Errorf("edge %s in %s carries %d records, the breakdown says %d",
					edge.Id, caseId, edge.EvidenceCount, byCase[edge.Id][caseId])
			}
			if got := caseCounts(edge.EvidenceByCase); len(got) != 1 || got[caseId] != edge.EvidenceCount {
				t.Errorf("edge %s in %s breaks down as %v", edge.Id, caseId, edge.EvidenceByCase)
			}
		}
	}
}

// エッジの詳細とノードの詳細も案件ごとの件数を持つ。案件で絞ったエッジの詳細は、その案件の
// 根拠だけを返す。
func TestGraphDetailsCountEvidenceByCase(t *testing.T) {
	graph := sharedCaseGraph(t)
	terminal, _ := core.TerminalNodeKey("T1")
	node, found := graph.NodeDetail(nodeIdOf(terminal))
	if !found {
		t.Fatal("the graph carries no terminal node T1")
	}
	if got := caseCounts(node.EvidenceByCase); got["baseline"] != 1 || got["challenge"] != 1 ||
		len(got) != 2 || int64(node.EvidenceCount) != got["baseline"]+got["challenge"] {
		t.Errorf("the terminal breaks down as %v with %d records", node.EvidenceByCase, node.EvidenceCount)
	}
	for _, count := range node.EdgeCounts {
		if err := count.Validate(); err != nil || count.EvidenceByCase == nil {
			t.Errorf("edge count %+v: %v", count, err)
		}
	}
	var edgeId string
	for _, edge := range graph.edges {
		if len(graph.casesOf(edge.evidence)) == 2 {
			edgeId = edge.id
			break
		}
	}
	whole, found := graph.EdgeDetail(edgeId, EdgeEvidenceFilter{})
	if !found {
		t.Fatalf("the graph carries no edge %s", edgeId)
	}
	if err := whole.Edge.Validate(); err != nil || len(whole.Edge.EvidenceByCase) != 2 {
		t.Fatalf("the edge detail breaks down as %v: %v", whole.Edge.EvidenceByCase, err)
	}
	narrowed, _ := graph.EdgeDetail(edgeId, EdgeEvidenceFilter{Case: "baseline"})
	if int64(len(narrowed.Evidence)) != narrowed.Edge.EvidenceCount ||
		narrowed.Edge.EvidenceCount != caseCounts(whole.Edge.EvidenceByCase)["baseline"] {
		t.Errorf("the baseline detail returns %d records and counts %d, the breakdown says %v",
			len(narrowed.Evidence), narrowed.Edge.EvidenceCount, whole.Edge.EvidenceByCase)
	}
	for _, evidence := range narrowed.Evidence {
		if evidence.RecordRef.SourceFileName != "left.log" {
			t.Errorf("the baseline detail returns a record of %s", evidence.RecordRef.SourceFileName)
		}
	}
}

// 案件を区別しない取り込みは内訳を出さない。
func TestGraphOmitsTheBreakdownWithoutCases(t *testing.T) {
	graph := NewGraph(caseImport(t,
		markIISource("left.log", caseFileSourceLeft, nil),
		markIISource("right.log", caseFileSourceRight, nil)), AllMatchConditions())
	for _, edge := range graph.Query(GraphQuery{Depth: 1}).Edges {
		if edge.EvidenceByCase != nil {
			t.Errorf("edge %s breaks down as %v without cases", edge.Id, edge.EvidenceByCase)
		}
	}
	terminal, _ := core.TerminalNodeKey("T1")
	if node, _ := graph.NodeDetail(nodeIdOf(terminal)); node.EvidenceByCase != nil {
		t.Errorf("the terminal breaks down as %v without cases", node.EvidenceByCase)
	}
	if graph.HasCase("baseline") {
		t.Error("the graph reports a case it does not carry")
	}
}

// 案件で絞った時系列は、その案件のレコードと収集元だけを返す。
func TestTimelineNarrowsToACase(t *testing.T) {
	graph := sharedCaseGraph(t)
	query := TimelineQuery{RecordFilter: RecordFilter{Case: "challenge"}}
	query.Validate()
	timeline := graph.Timeline(query)
	if len(timeline.Entries) == 0 {
		t.Fatal("the challenge timeline is empty")
	}
	for _, entry := range timeline.Entries {
		if entry.RecordRef.SourceFileName != "right.log" {
			t.Errorf("the challenge timeline returns a record of %s", entry.RecordRef.SourceFileName)
		}
	}
	if len(timeline.SourceCoverages) != 1 || timeline.SourceCoverages[0].SourceFileName != "right.log" {
		t.Errorf("the challenge timeline covers %+v, want right.log only", timeline.SourceCoverages)
	}
}

// 記録の応答が補う接続元の端末は、そのレコードの案件の割当だけから導く。
func TestFieldsBuilderDerivesTheTerminalOnlyWithinACase(t *testing.T) {
	markII, err := os.ReadFile("../internal/testdata/run/graph-markii.log")
	if err != nil {
		t.Fatal(err)
	}
	squid, err := os.ReadFile("../internal/testdata/run/graph-squid.log")
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		name        string
		endpoint    *string
		proxy       *string
		wantDerived bool
	}{
		{"no case", nil, nil, true},
		{"one case", caseOf("challenge"), caseOf("challenge"), true},
		{"two cases", caseOf("baseline"), caseOf("challenge"), false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result := caseImport(t,
				markIISource("endpoint.log", string(markII), testCase.endpoint),
				caseSource{name: "proxy.log", format: string(SquidFormatKey), content: string(squid),
					caseId: testCase.proxy})
			builder := NewFieldsBuilder(result)
			derived := 0
			for _, record := range result.publications[1].records {
				if record.Semantics == nil || record.Semantics.Endpoint == nil {
					continue
				}
				fields, err := builder.Build(record)
				if err != nil {
					t.Fatal(err)
				}
				for _, field := range fieldsWithSemantic(fields, core.SemanticKeyTerminalId) {
					if field.Name == clientTerminalFieldName && field.Text != nil &&
						field.Text.ValueState == core.ValueStateDerived {
						derived++
					}
				}
			}
			if (derived > 0) != testCase.wantDerived {
				t.Errorf("the proxy records carry %d derived terminals, want present=%t", derived, testCase.wantDerived)
			}
		})
	}
}

// 2 つの案件が同じ端末を記録しても、端末のノードは 1 つで、根拠は両方の案件のレコードを持つ。
func TestGraphSharesTheNodeAcrossCases(t *testing.T) {
	result := caseImport(t,
		markIISource("left.log", caseFileSourceLeft, caseOf("baseline")),
		markIISource("right.log", caseFileSourceRight, caseOf("challenge")))
	graph := NewGraph(result, AllMatchConditions())
	terminal, named := core.TerminalNodeKey("T1")
	if !named {
		t.Fatal("the terminal key is not buildable")
	}
	at, found := graph.nodeAt[nodeIdOf(terminal)]
	if !found {
		t.Fatal("the graph carries no terminal node T1")
	}
	cases := graph.casesOf(graph.nodes[at].evidence)
	if len(cases) != 2 || !contains(cases, "baseline") || !contains(cases, "challenge") {
		t.Errorf("the terminal node holds the evidence of the cases %v, want baseline and challenge", cases)
	}
}
