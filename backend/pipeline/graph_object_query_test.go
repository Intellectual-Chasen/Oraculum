// in-package test: 文字列の条件をレコード単位で判定することと、対象の粒度と、端末の条件と、
// ノードの数の上限を、レコードで確かめる。
package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// searchRecordsGraph は検索の条件を確かめるグラフを組む。
//
// 行 1 と行 2 は端末 T1 のプロセス P1 を指し、文字列 203.0.113.9 と powershell を別の行に
// 持つ。行 3 は端末 T2 のプロセス P2 を指し、2 つの文字列を同じ行に持つ。行 4 は端末を名乗らず、
// 203.0.113.9 だけを持つ。行 5 は端末 T1 のプロセス P3 を指し、どちらの文字列も持たない。
func searchRecordsGraph(t *testing.T) Graph {
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
			syntheticField(t, "cmd", core.SemanticKeyProcessCommandLine, "a.exe 203.0.113.9")),
		record(2,
			syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
			syntheticField(t, "psGUID", core.SemanticKeyProcessId, "{P1}"),
			syntheticField(t, "cmd", core.SemanticKeyProcessCommandLine, "powershell -c get")),
		record(3,
			syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T2"),
			syntheticField(t, "psGUID", core.SemanticKeyProcessId, "{P2}"),
			syntheticField(t, "cmd", core.SemanticKeyProcessCommandLine,
				"b.exe 203.0.113.9 powershell")),
		record(4,
			syntheticField(t, "note", "", "203.0.113.9")),
		record(5,
			syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
			syntheticField(t, "psGUID", core.SemanticKeyProcessId, "{P3}"),
			syntheticField(t, "cmd", core.SemanticKeyProcessCommandLine, "c.exe")),
	})
}

// labelsOf は応答のノードを「種別:識別鍵の最後の値」の文字列で返す。レコードのノードは行番号である。
func labelsOf(nodes []core.SubgraphNode) []string {
	labels := make([]string, 0, len(nodes))
	for _, node := range nodes {
		last := node.Identity[len(node.Identity)-1].Value
		labels = append(labels, string(node.Kind)+":"+last)
	}
	slices.Sort(labels)
	return labels
}

// terminalIdOf は識別鍵の値が value の端末のノードの識別子を返す。
func terminalIdOf(t *testing.T, graph Graph, value string) string {
	t.Helper()
	subgraph := graph.Query(GraphQuery{NodeKinds: []core.NodeKind{core.NodeKindTerminal}})
	for _, node := range subgraph.Nodes {
		if node.Identity[len(node.Identity)-1].Value == value {
			return node.Id
		}
	}
	t.Fatalf("the graph carries no terminal %q", value)
	return ""
}

func TestQueryJudgesTheTextConditionsRecordByRecord(t *testing.T) {
	graph := searchRecordsGraph(t)
	for _, testCase := range []struct {
		name  string
		query GraphQuery
		want  []string
	}{
		{
			// 2 つの文字列を同じ行に持つのは行 3 だけである。行 1 と行 2 は同じプロセスを
			// 指すが、文字列を別の行に持つ。
			name: "every token in the record",
			query: GraphQuery{NodeKinds: []core.NodeKind{core.NodeKindRecord},
				ValueContains: []string{"203.0.113.9", "powershell"}},
			want: []string{"record:3"},
		},
		{
			name: "a token the record must not carry",
			query: GraphQuery{NodeKinds: []core.NodeKind{core.NodeKindRecord},
				ValueContains: []string{"203.0.113.9"}, ValueExcludes: []string{"powershell"}},
			want: []string{"record:1", "record:4"},
		},
		{
			// 対象の粒度でも、判定は行ごとである。P1 は 2 つの文字列を別の行で持つため合わない。
			name: "the object granularity keeps the record-level judgement",
			query: GraphQuery{Granularity: core.GraphGranularityObject,
				NodeKinds:     []core.NodeKind{core.NodeKindProcess},
				ValueContains: []string{"203.0.113.9", "powershell"}},
			want: []string{"process:{P2}"},
		},
		{
			name: "the object granularity lifts the matched records to their objects",
			query: GraphQuery{Granularity: core.GraphGranularityObject,
				ValueContains: []string{"203.0.113.9"}},
			// 端末は全レコードに指されるため、レコードの文字列では起点に数えない。
			want: []string{"process:{P1}", "process:{P2}"},
		},
		{
			name: "the object granularity without a text condition",
			query: GraphQuery{Granularity: core.GraphGranularityObject,
				NodeKinds: []core.NodeKind{core.NodeKindProcess}},
			want: []string{"process:{P1}", "process:{P2}", "process:{P3}"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			subgraph := graph.Query(testCase.query)
			if got := labelsOf(subgraph.Nodes); !slices.Equal(got, testCase.want) {
				t.Errorf("the query matched %v, want %v", got, testCase.want)
			}
			if subgraph.MatchedNodeCount != len(testCase.want) {
				t.Errorf("matchedNodeCount=%d, want %d",
					subgraph.MatchedNodeCount, len(testCase.want))
			}
			sum := 0
			for _, kind := range subgraph.MatchedKinds {
				sum += int(kind.Count)
			}
			if sum != subgraph.MatchedNodeCount {
				t.Errorf("the matched kinds sum to %d, want matchedNodeCount %d",
					sum, subgraph.MatchedNodeCount)
			}
		})
	}
}

func TestQueryNarrowsTheRecordsByTheTerminalTheyName(t *testing.T) {
	graph := searchRecordsGraph(t)
	terminal := terminalIdOf(t, graph, "T1")
	for _, testCase := range []struct {
		name     string
		terminal string
		want     []string
	}{
		// 行 4 は端末を名乗らないため、端末を与えた要求に入らない。
		{"a terminal", terminal, []string{"record:1"}},
		{"no terminal", "", []string{"record:1", "record:3", "record:4"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			subgraph := graph.Query(GraphQuery{
				NodeKinds:     []core.NodeKind{core.NodeKindRecord},
				ValueContains: []string{"203.0.113.9"},
				RecordFilter:  RecordFilter{Terminal: testCase.terminal},
			})
			if got := labelsOf(subgraph.Nodes); !slices.Equal(got, testCase.want) {
				t.Errorf("the query matched %v, want %v", got, testCase.want)
			}
		})
	}
	if !graph.IsTerminalNode(terminal) {
		t.Errorf("IsTerminalNode(%q) is false, want the terminal", terminal)
	}
	process := graph.Query(GraphQuery{NodeKinds: []core.NodeKind{core.NodeKindProcess}}).Nodes[0].Id
	if graph.IsTerminalNode(process) || graph.IsTerminalNode("n:terminal:absent") {
		t.Error("IsTerminalNode accepts a process or an unknown identifier")
	}
}

// 対象の粒度で近傍を辿ると、レコードのノードとレコードが対象を指す関係を出さない。
func TestQueryAtTheObjectGranularityCarriesNoRecord(t *testing.T) {
	graph := searchRecordsGraph(t)
	for _, granularity := range []core.GraphGranularity{
		core.GraphGranularityRecord, core.GraphGranularityObject,
	} {
		t.Run(string(granularity), func(t *testing.T) {
			subgraph := graph.Query(GraphQuery{Granularity: granularity, Depth: 1,
				NodeKinds: []core.NodeKind{core.NodeKindProcess}})
			carriesRecord := slices.ContainsFunc(subgraph.Nodes,
				func(node core.SubgraphNode) bool { return node.Kind == core.NodeKindRecord })
			namesObject := slices.ContainsFunc(subgraph.Edges,
				func(edge core.GraphEdge) bool { return edge.Kind == core.EdgeKindRecordNamesObject })
			wantRecords := granularity == core.GraphGranularityRecord
			if carriesRecord != wantRecords || namesObject != wantRecords {
				t.Errorf("records=%v naming edges=%v, want %v", carriesRecord, namesObject,
					wantRecords)
			}
		})
	}
}

// 対象の粒度で文字列の条件を持つ要求は、文字列を満たすレコードが作った関係だけを辿る。
// 文字列の条件を持たない要求は、端末から、その端末で動いた全てのプロセスへ辿る。
func TestQueryAtTheObjectGranularityFollowsTheSearchedRecords(t *testing.T) {
	graph := searchRecordsGraph(t)
	for _, testCase := range []struct {
		name     string
		contains []string
		want     []string
	}{
		{"a text condition", []string{"203.0.113.9"},
			[]string{"process:{P1}", "process:{P2}", "terminal:T1", "terminal:T2"}},
		{"no text condition", nil,
			[]string{"process:{P1}", "process:{P2}", "process:{P3}", "terminal:T1", "terminal:T2"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			subgraph := graph.Query(GraphQuery{Granularity: core.GraphGranularityObject,
				Depth: 1, ValueContains: testCase.contains,
				NodeKinds: []core.NodeKind{core.NodeKindProcess, core.NodeKindTerminal}})
			if got := labelsOf(subgraph.Nodes); !slices.Equal(got, testCase.want) {
				t.Errorf("the query carries %v, want %v", got, testCase.want)
			}
		})
	}
}

// 文字列がどこかにあるかの判定は、対象の粒度ではレコード単位で行う。P1 は 2 つの文字列を
// 別の行で持つため、対象の属性を集めて見ると文字列があると読み違える。
func TestHasValueMatchAnywhereJudgesRecordByRecordAtTheObjectGranularity(t *testing.T) {
	graph := searchRecordsGraph(t)
	for _, testCase := range []struct {
		name        string
		granularity core.GraphGranularity
		contains    []string
		want        bool
	}{
		// a.exe は行 1 に、get は行 2 にだけある。P1 の属性は両方を集める。
		{"tokens split over two records, object", core.GraphGranularityObject,
			[]string{"a.exe", "get"}, false},
		{"tokens split over two records, record", core.GraphGranularityRecord,
			[]string{"a.exe", "get"}, true},
		{"tokens in one record, object", core.GraphGranularityObject,
			[]string{"b.exe", "powershell"}, true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := graph.HasValueMatchAnywhere(GraphQuery{
				Granularity: testCase.granularity, ValueContains: testCase.contains,
			})
			if got != testCase.want {
				t.Errorf("HasValueMatchAnywhere=%v, want %v", got, testCase.want)
			}
		})
	}
}

// 部分グラフのノードの数が上限を超えるとき、合ったノードと関係の種別ごとの本数を返し、端点と
// エッジの本体を返さない。上限に収まるときは全部を返す。
func TestQueryBeyondTheNodeLimitReturnsTheMatchedNodes(t *testing.T) {
	graph := graphOf(t)
	terminals := []core.NodeKind{core.NodeKindTerminal}
	whole := graph.Query(GraphQuery{NodeKinds: terminals, Depth: 1})
	matched, subgraphNodes := whole.MatchedNodeCount, len(whole.Nodes)
	if matched == 0 || subgraphNodes <= matched+1 || len(whole.Edges) == 0 {
		t.Fatalf("the fixture yields %d matched nodes among %d, want endpoints beyond them",
			matched, subgraphNodes)
	}
	if whole.SubgraphNodeCount != subgraphNodes || whole.NodeLimitExceeded || whole.EdgeKindCounts != nil {
		t.Errorf("without a limit: count=%d exceeded=%v kinds=%v, want %d and no limit",
			whole.SubgraphNodeCount, whole.NodeLimitExceeded, whole.EdgeKindCounts, subgraphNodes)
	}
	beyond := graph.Query(GraphQuery{NodeKinds: terminals, Depth: 1, NodeLimit: subgraphNodes - 1})
	if !beyond.NodeLimitExceeded || beyond.SubgraphNodeCount != subgraphNodes {
		t.Fatalf("exceeded=%v count=%d, want exceeded with %d",
			beyond.NodeLimitExceeded, beyond.SubgraphNodeCount, subgraphNodes)
	}
	if len(beyond.Nodes) != matched || len(beyond.Edges) != 0 || beyond.EdgeCount != 0 {
		t.Errorf("nodes=%d edges=%d edgeCount=%d, want the %d matched nodes and no edge",
			len(beyond.Nodes), len(beyond.Edges), beyond.EdgeCount, matched)
	}
	for _, node := range beyond.Nodes {
		if node.Selection != core.NodeSelectionMatched {
			t.Errorf("the node %q is %s beyond the limit, want matched", node.Id, node.Selection)
		}
	}
	var counted int64
	for _, kind := range beyond.EdgeKindCounts {
		counted += kind.Count
	}
	if counted != int64(len(whole.Edges)) || beyond.MatchedNodeCount != matched ||
		!slices.Equal(beyond.MatchedKinds, whole.MatchedKinds) {
		t.Errorf("edge kinds count %d, matched=%d kinds=%v, want %d %d %v", counted,
			beyond.MatchedNodeCount, beyond.MatchedKinds, len(whole.Edges), matched, whole.MatchedKinds)
	}
	within := graph.Query(GraphQuery{NodeKinds: terminals, Depth: 1, NodeLimit: subgraphNodes})
	if within.NodeLimitExceeded || len(within.Nodes) != subgraphNodes || len(within.Edges) != len(whole.Edges) {
		t.Errorf("exceeded=%v nodes=%d edges=%d, want the whole subgraph within the limit",
			within.NodeLimitExceeded, len(within.Nodes), len(within.Edges))
	}
}
