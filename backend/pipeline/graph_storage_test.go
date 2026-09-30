// in-package test: 非公開の slice の容量を直に読む。
package pipeline

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// **容量が長さを超える slice だけを移し、nil と要素数 0 の集合を区別したまま返す。**
func TestExactSliceReleasesOnlySpareCapacity(t *testing.T) {
	spare := make([]int, 2, 8)
	spare[0], spare[1] = 3, 5
	moved := exactSlice(spare)
	if !slices.Equal(moved, []int{3, 5}) || cap(moved) != len(moved) {
		t.Errorf("exactSlice(spare) = %v with capacity %d, want [3 5] with capacity 2",
			moved, cap(moved))
	}
	moved[0] = 7
	if spare[0] != 3 {
		t.Errorf("writing the moved slice changed the original to %v", spare)
	}

	exact := []int{3, 5}
	if kept := exactSlice(exact); &kept[0] != &exact[0] {
		t.Error("exactSlice moved a slice whose capacity equals its length, want the same array")
	}
	if got := exactSlice([]int(nil)); got != nil {
		t.Errorf("exactSlice(nil) = %#v, want nil", got)
	}
	if got := exactSlice(make([]int, 0, 4)); got == nil || len(got) != 0 {
		t.Errorf("exactSlice(an empty set) = %#v, want an empty non-nil set", got)
	}
}

// storageTestField は文字列の項目を組む。
func storageTestField(t *testing.T, name string, semantic core.SemanticKey, text string) core.RecordField {
	t.Helper()
	value, err := core.NewRawValue(core.ValueStatePresent, text)
	if err != nil {
		t.Fatal(err)
	}
	field, err := core.NewTextField(name, semantic, value)
	if err != nil {
		t.Fatal(err)
	}
	return field
}

// **項目を指す並びは graphFieldsOf と同じ項目を同じ順に持ち、取り込み結果の項目そのものを
// 指す。** 意味付けの結果に同じ語彙の項目がある端末の項目と、語彙の項目を持たない端末の
// 項目は足さない。
func TestGraphFieldRefsPointAtTheImportedFields(t *testing.T) {
	record := RecordEntry{
		Semantics: &RecordSemantics{Fields: []core.RecordField{
			storageTestField(t, "host", core.SemanticKeyTerminalHostname, "pc01.example.test"),
			storageTestField(t, "pid", core.SemanticKeyProcessPid, "4242"),
		}},
		Terminal: []core.RecordField{
			storageTestField(t, "wsName", core.SemanticKeyTerminalHostname, "pc02.example.test"),
			storageTestField(t, "wsIp", core.SemanticKeyTerminalIpAddress, "192.0.2.10"),
			storageTestField(t, "note", "", "synthetic"),
		},
	}
	refs := graphFieldRefsOf(record)
	fields := graphFieldsOf(record)
	if len(refs) != len(fields) {
		t.Fatalf("graphFieldRefsOf returned %d fields, graphFieldsOf %d", len(refs), len(fields))
	}
	for index, ref := range refs {
		if !reflect.DeepEqual(*ref, fields[index]) {
			t.Errorf("the field at %d is %+v, graphFieldsOf has %+v", index, *ref, fields[index])
		}
	}
	want := []*core.RecordField{
		&record.Semantics.Fields[0], &record.Semantics.Fields[1], &record.Terminal[1],
	}
	if !slices.Equal(refs, want) {
		t.Errorf("the refs point at %v, want the imported fields %v", refs, want)
	}
	if got := graphFieldRefsOf(RecordEntry{}); len(got) != 0 {
		t.Errorf("a record without fields yields %d refs, want none", len(got))
	}
}

// **組み終えた観測の層と選択ごとのグラフは、足していくときに伸ばした容量を持たない。**
func TestBuiltGraphsHoldNoSpareCapacity(t *testing.T) {
	observed := NewObservedGraph(graphResult(t))
	selected := observed.WithCandidateEdges(graphResult(t), AllMatchConditions())
	for name, graph := range map[string]Graph{"observed": observed, "selected": selected} {
		if cap(graph.edges) != len(graph.edges) || cap(graph.nodes) != len(graph.nodes) {
			t.Errorf("%s: edges %d/%d and nodes %d/%d, want the capacity equal to the length",
				name, len(graph.edges), cap(graph.edges), len(graph.nodes), cap(graph.nodes))
		}
		for index, node := range graph.nodes {
			outgoing := graph.adjacency[index].outgoing
			if cap(outgoing) != len(outgoing) || cap(node.evidence) != len(node.evidence) {
				t.Errorf("%s: the node %q holds spare capacity", name, node.id)
			}
		}
	}
	if len(selected.edges) == selected.observedEdgeCount {
		t.Fatal("the selection derives no candidate edge, want the fixture to derive one")
	}
}
