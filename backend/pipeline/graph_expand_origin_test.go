// in-package test: 条件を起点だけに当てる要求と、中継の段階の端末からレコードが対象を指す関係を辿らないことを、
// レコードで確かめる。
package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/attackrules"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// expandOriginGraph は、端末 T1 の 2 件のレコードを持つグラフを組む。行 1 はプロセス P1 の
// 起動 (分類 proc・動作 start、文字列 a.exe)、行 2 はプロセス P2 の通信 (分類 net・動作 conn、
// 文字列 b.exe) である。
func expandOriginGraph(t *testing.T) Graph {
	t.Helper()
	record := func(line int64, fields ...core.RecordField) RecordEntry {
		return RecordEntry{
			Locator: core.RecordLocator{
				SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
				LineNumber: &line,
			},
			Semantics: &RecordSemantics{
				ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
				Fields:          fields,
			},
		}
	}
	return graphOfRecords(t, []RecordEntry{
		record(1,
			syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
			syntheticField(t, "psGUID", core.SemanticKeyProcessId, "{P1}"),
			syntheticField(t, "category", core.SemanticKeyEventCategory, "proc"),
			syntheticField(t, "action", core.SemanticKeyEventAction, "start"),
			syntheticField(t, "cmd", core.SemanticKeyProcessCommandLine, "a.exe")),
		record(2,
			syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
			syntheticField(t, "psGUID", core.SemanticKeyProcessId, "{P2}"),
			syntheticField(t, "category", core.SemanticKeyEventCategory, "net"),
			syntheticField(t, "action", core.SemanticKeyEventAction, "conn"),
			syntheticField(t, "cmd", core.SemanticKeyProcessCommandLine, "b.exe")),
	})
}

// 起点の条件 (文字列・事象の種別) を起点だけに当てると、広げる段階は条件に合わないレコードが
// 作った関係も辿る。当てないと、起点と同じ条件で段階が絞られて P2 に届かない。
func TestQueryAppliesTheConditionsToTheOriginsOnly(t *testing.T) {
	graph := expandOriginGraph(t)
	for _, testCase := range []struct {
		name   string
		filter GraphQuery
	}{
		{"a text condition", GraphQuery{ValueContains: []string{"a.exe"}}},
		{"an event kind", GraphQuery{RecordFilter: RecordFilter{
			EventCategory: "proc", EventAction: "start"}}},
	} {
		for _, originOnly := range []bool{false, true} {
			query := testCase.filter
			query.Granularity = core.GraphGranularityObject
			query.NodeKinds = []core.NodeKind{core.NodeKindProcess, core.NodeKindTerminal}
			query.Depth = 2
			query.ConditionsOnOriginsOnly = originOnly
			subgraph := graph.Query(query)
			got := labelsOf(subgraph.Nodes)
			if reached := slices.Contains(got, "process:{P2}"); reached != originOnly {
				t.Errorf("%s, origins only=%v: the query carries %v, want P2 reached=%v",
					testCase.name, originOnly, got, originOnly)
			}
			if subgraph.MatchedNodeCount != 1 {
				t.Errorf("%s, origins only=%v: matchedNodeCount=%d, want the origin P1",
					testCase.name, originOnly, subgraph.MatchedNodeCount)
			}
			// 広げた段階のエッジの根拠の件数は、段階の条件で数える。どのエッジも 1 件のレコードが作った。
			for _, edge := range subgraph.Edges {
				if edge.EvidenceCount != 1 {
					t.Errorf("%s, origins only=%v: edge %s carries %d records, want 1",
						testCase.name, originOnly, edge.Id, edge.EvidenceCount)
				}
			}
		}
	}
}

// 端末は全レコードに指されるため、根拠のレコードが事象の種別または文字列の条件を満たすことで
// 端末を起点に数えない。どちらの粒度でも同じである。
func TestQueryDoesNotCountATerminalAsAnOriginByItsRecords(t *testing.T) {
	graph := expandOriginGraph(t)
	from := uint64(1)
	for _, granularity := range []core.GraphGranularity{
		core.GraphGranularityObject, core.GraphGranularityRecord,
	} {
		for _, testCase := range []struct {
			name     string
			filter   GraphQuery
			terminal bool
		}{
			{"an event kind", GraphQuery{RecordFilter: RecordFilter{
				EventCategory: "proc", EventAction: "start"}}, false},
			{"an event action range only", GraphQuery{RecordFilter: RecordFilter{
				EventActionFrom: &from}}, false},
			{"an event kind with the terminal named", GraphQuery{
				NodeIds:      []string{terminalIdOf(t, graph, "T1")},
				RecordFilter: RecordFilter{EventCategory: "proc", EventAction: "start"}}, true},
			{"a text of a record", GraphQuery{ValueContains: []string{"a.exe"}}, false},
			{"no condition", GraphQuery{}, true},
		} {
			query := testCase.filter
			query.Granularity = granularity
			query.NodeKinds = []core.NodeKind{core.NodeKindTerminal}
			got := labelsOf(graph.Query(query).Nodes)
			if slices.Contains(got, "terminal:T1") != testCase.terminal {
				t.Errorf("%s, %s: the origins are %v, want terminal T1=%v",
					granularity, testCase.name, got, testCase.terminal)
			}
		}
	}
}

// 端末自身の属性 (ホスト名) が文字列の条件を満たせば、文字列を満たす根拠のレコードが無くても
// 端末を起点に数える。行 1 はホスト名と a.exe を持ち、行 2 はどちらも持たない。含む文字列
// host-a と含まない文字列 a.exe を両方満たすレコードは無い。
func TestQueryCountsATerminalWhoseOwnAttributeMatchesTheText(t *testing.T) {
	record := func(line int64, fields ...core.RecordField) RecordEntry {
		return RecordEntry{
			Locator: core.RecordLocator{
				SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
				LineNumber: &line,
			},
			Semantics: &RecordSemantics{
				ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
				Fields:          fields,
			},
		}
	}
	graph := graphOfRecords(t, []RecordEntry{
		record(1,
			syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
			syntheticField(t, "host", core.SemanticKeyTerminalHostname, "host-a"),
			syntheticField(t, "cmd", core.SemanticKeyProcessCommandLine, "a.exe")),
		record(2,
			syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
			syntheticField(t, "cmd", core.SemanticKeyProcessCommandLine, "b.exe")),
	})
	for _, granularity := range []core.GraphGranularity{
		core.GraphGranularityObject, core.GraphGranularityRecord,
	} {
		got := labelsOf(graph.Query(GraphQuery{
			Granularity: granularity, NodeKinds: []core.NodeKind{core.NodeKindTerminal},
			ValueContains: []string{"host-a"}, ValueExcludes: []string{"a.exe"},
		}).Nodes)
		if !slices.Contains(got, "terminal:T1") {
			t.Errorf("%s: the origins are %v, want terminal T1", granularity, got)
		}
	}
}

// 段階の要求は文字列と事象の種別の条件だけを外し、期間・案件・端末と関係の種別を残す。
func TestStepQueryKeepsTheOtherConditions(t *testing.T) {
	from := uint64(1)
	query := GraphQuery{
		ValueContains: []string{"a"}, ValueExcludes: []string{"b"},
		EdgeKinds: []core.EdgeKind{core.EdgeKindRanOn},
		RecordFilter: RecordFilter{EventCategory: "proc", EventAction: "start",
			EventActionFrom: &from, EventActionTo: &from, Case: "case-a", Terminal: "n:terminal:x"},
		ConditionsOnOriginsOnly: true,
	}
	step := query.stepQuery()
	if step.ValueContains != nil || step.ValueExcludes != nil || step.EventCategory != "" ||
		step.EventAction != "" || step.EventActionFrom != nil || step.EventActionTo != nil {
		t.Errorf("the step query keeps a text or event kind condition: %+v", step)
	}
	if step.Case != "case-a" || step.Terminal != "n:terminal:x" ||
		!slices.Equal(step.EdgeKinds, query.EdgeKinds) {
		t.Errorf("the step query drops a condition it must keep: %+v", step)
	}
	query.ConditionsOnOriginsOnly = false
	if step := query.stepQuery(); step.EventCategory != "proc" || len(step.ValueContains) != 1 {
		t.Errorf("the step query without the option drops a condition: %+v", step)
	}
}

// 条件を起点だけに当てた要求では、調べる順序の入力と ATT&CK 候補の根拠も、段階で届いた関係を
// 段階の条件で絞る。図のエッジの根拠の件数と食い違わない。
func TestOrderInputAndAttackCandidatesFilterTheStepsLikeTheQuery(t *testing.T) {
	graph := expandOriginGraph(t)
	query := GraphQuery{Granularity: core.GraphGranularityObject,
		NodeKinds: []core.NodeKind{core.NodeKindProcess, core.NodeKindTerminal}, Depth: 2,
		RecordFilter:            RecordFilter{EventCategory: "proc", EventAction: "start"},
		ConditionsOnOriginsOnly: true}

	input := graph.orderInputOf(query)
	if len(input.objects) != 3 || len(input.pairs) != 2 {
		t.Errorf("the order input carries %d objects and %d pairs, want P1, P2, T1 and 2 pairs",
			len(input.objects), len(input.pairs))
	}
	// 端末 T1 は起点に入らず段階で届くため、段階の条件で両方の行を持つ。
	for i, index := range input.objects {
		want := 1
		if graph.nodes[index].key.Kind == core.NodeKindTerminal {
			want = 2
		}
		if len(input.records[i]) != want {
			t.Errorf("object %s carries %d records, want %d",
				graph.nodes[index].id, len(input.records[i]), want)
		}
	}
	for _, pair := range input.pairs {
		if len(pair.records) == 0 {
			t.Errorf("the pair %d-%d carries no record", pair.a, pair.b)
		}
	}

	rule := attackrules.Rule{ID: "synthetic", Title: "Synthetic", Description: "synthetic rule",
		References: []string{"https://example.test/synthetic"},
		Attack:     []attackrules.AttackRef{{ID: "T1055", Basis: attackrules.AttackBasisInferred}},
		Variants: []attackrules.Variant{{ID: "default",
			Pattern: attackrules.Pattern{Nodes: map[string]attackrules.PatternNode{
				"process": {Kind: string(core.NodeKindProcess)}, "terminal": {Kind: string(core.NodeKindTerminal)},
			}, Edges: map[string]attackrules.PatternEdge{
				"ran_on": {Kind: string(core.EdgeKindRanOn), From: "process", To: "terminal"},
			}, Evidence: attackrules.Evidence{Required: []string{"ran_on"}}}}}}
	matched, err := graph.AttackRuleMatches(query, []attackrules.Rule{rule})
	if err != nil {
		t.Fatal(err)
	}
	matches := matched.Matches
	if len(matches) != 1 {
		t.Fatalf("the scoped query matched %d ran_on topologies, want one", len(matches))
	}
	for _, match := range matches {
		edge := match.Edges[0]
		if int64(len(edge.Evidence)) != edge.Edge.EvidenceCount {
			t.Errorf("edge %s carries %d evidence items for a count of %d",
				edge.Edge.Id, len(edge.Evidence), edge.Edge.EvidenceCount)
		}
	}
}

// 起点以外の端末からは、レコードが対象を指す関係を辿らない。端末は多くのレコードに
// 指され、辿ると全レコードが入る。起点に選んだ端末からは辿る。
func TestQueryDoesNotFollowTheNamingEdgesOfARelayTerminal(t *testing.T) {
	graph := expandOriginGraph(t)
	fromRecord := graph.Query(GraphQuery{ValueContains: []string{"a.exe"},
		NodeKinds: []core.NodeKind{core.NodeKindRecord}, Depth: 2,
		ConditionsOnOriginsOnly: true})
	got := labelsOf(fromRecord.Nodes)
	if slices.Contains(got, "record:2") || !slices.Contains(got, "terminal:T1") {
		t.Errorf("the query from record 1 carries %v, want terminal T1 without record 2", got)
	}

	fromTerminal := graph.Query(GraphQuery{NodeIds: []string{terminalIdOf(t, graph, "T1")},
		Depth: 1})
	got = labelsOf(fromTerminal.Nodes)
	if !slices.Contains(got, "record:1") || !slices.Contains(got, "record:2") {
		t.Errorf("the query from terminal T1 carries %v, want both records", got)
	}
}
