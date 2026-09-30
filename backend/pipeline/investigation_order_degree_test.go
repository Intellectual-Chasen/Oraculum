// in-package test: レコードからグラフを組み、次数の手法の値と順位を確かめる。
package pipeline

import (
	"maps"
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// degreeTestRecords は、プロセス {P1} を中心に 3 つのファイルが並ぶ星形の 4 件と、端末 T2 だけを
// 指す 1 件である。T2 は他の対象と組を作らない。
func degreeTestRecords(t *testing.T) []RecordEntry {
	t.Helper()
	line := int64(5)
	lone := RecordEntry{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber, LineNumber: &line,
		},
		Semantics: &RecordSemantics{
			Fields:          []core.RecordField{syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T2")},
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
		},
	}
	return []RecordEntry{
		orderTestRecord(t, 1, "{P1}", `c:\a.txt`),
		orderTestRecord(t, 2, "{P1}", `c:\b.txt`),
		orderTestRecord(t, 3, "{P1}", `c:\c.txt`),
		orderTestRecord(t, 4, "{P2}", `c:\a.txt`),
		lone,
	}
}

// degreesOf は並びの対象ごとに、識別の最後の値から次数を探す表を返す。
func degreesOf(t *testing.T, order InvestigationOrder) map[string]float64 {
	t.Helper()
	degrees := make(map[string]float64)
	for _, kind := range order.Kinds {
		for _, entry := range kind.Entries {
			if entry.Value == nil {
				t.Fatalf("entry %+v has no value, want every object to have a degree", entry)
			}
			identity := entry.Node.Identity
			degrees[identity[len(identity)-1].Value] = *entry.Value
		}
	}
	return degrees
}

func TestInvestigationOrderDegreeCountsDistinctPartners(t *testing.T) {
	graph := orderTestGraph(t, degreeTestRecords(t)...)
	order := investigationOrderOf(t, graph, GraphQuery{}, InvestigationOrderDegree)

	if len(order.Parameters) != 0 {
		t.Errorf("parameters = %+v, want none", order.Parameters)
	}
	want := map[string]float64{
		"T1": 5, "T2": 0, "{P1}": 4, "{P2}": 2, `c:\a.txt`: 3, `c:\b.txt`: 2, `c:\c.txt`: 2,
	}
	if got := degreesOf(t, order); !maps.Equal(got, want) {
		t.Errorf("degrees = %v, want %v", got, want)
	}
	if got, want := orderedIdentitiesOf(t, order, core.NodeKindProcess), []orderedIdentity{
		{"{P1}", 1, 1}, {"{P2}", 2, 1},
	}; !slices.Equal(got, want) {
		t.Errorf("processes = %+v, want the star center first", got)
	}
	if got, want := orderedIdentitiesOf(t, order, core.NodeKindFile), []orderedIdentity{
		{`c:\a.txt`, 1, 1}, {`c:\b.txt`, 2, 2}, {`c:\c.txt`, 2, 2},
	}; !slices.Equal(got, want) {
		t.Errorf("files = %+v, want %+v", got, want)
	}
	if got, want := orderedIdentitiesOf(t, order, core.NodeKindTerminal), []orderedIdentity{
		{"T1", 1, 1}, {"T2", 2, 1},
	}; !slices.Equal(got, want) {
		t.Errorf("terminals = %+v, want the terminal without pairs last", got)
	}
}
