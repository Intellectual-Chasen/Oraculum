package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// エッジの根拠のレコードは、そのエッジと両端のノードを返す。返すエッジはどれもそのレコードを
// 根拠に持ち、返すノードはそのレコードを根拠に持つか、返すエッジの端点である。識別子は重複しない。
func TestRecordGraphElementsReturnsWhatTheRecordIsEvidenceOf(t *testing.T) {
	graph := graphOf(t)
	edgeAt := slices.IndexFunc(graph.edges, func(edge graphEdge) bool {
		return len(edge.evidence) > 0
	})
	if edgeAt < 0 {
		t.Fatal("the fixture has no edge with evidence")
	}
	edge := graph.edges[edgeAt]
	at := edge.evidence[0]
	elements := graph.RecordGraphElements(graph.records[at].locator)

	if !slices.Contains(elements.EdgeIds, edge.id) {
		t.Fatalf("edgeIds=%v lack the edge %q the record is evidence of", elements.EdgeIds, edge.id)
	}
	endpoints := map[string]struct{}{}
	for _, id := range elements.EdgeIds {
		index := slices.IndexFunc(graph.edges, func(candidate graphEdge) bool { return candidate.id == id })
		if !slices.Contains(graph.edges[index].evidence, at) {
			t.Errorf("the edge %q does not have the record as evidence", id)
		}
		endpoints[graph.nodes[graph.edges[index].source].id] = struct{}{}
		endpoints[graph.nodes[graph.edges[index].target].id] = struct{}{}
	}
	for id := range endpoints {
		if !slices.Contains(elements.NodeIds, id) {
			t.Errorf("nodeIds lack the endpoint %q", id)
		}
	}
	for _, id := range elements.NodeIds {
		_, endpoint := endpoints[id]
		if !endpoint && !slices.Contains(graph.nodes[graph.nodeAt[id]].evidence, at) {
			t.Errorf("the node %q neither has the record as evidence nor ends a returned edge", id)
		}
	}
	for _, ids := range [][]string{elements.NodeIds, elements.EdgeIds} {
		if len(slices.Compact(slices.Sorted(slices.Values(ids)))) != len(ids) {
			t.Errorf("the identifiers %v repeat", ids)
		}
	}
}

// グラフの根拠に入っていないレコードは、空のノードとエッジを返す。
func TestRecordGraphElementsIsEmptyForARecordOutsideTheGraph(t *testing.T) {
	sequence := int64(1)
	elements := graphOf(t).RecordGraphElements(core.RecordLocator{
		SourceId:       "source-absent",
		PositionKind:   core.PositionKindSequenceNumber,
		SequenceNumber: &sequence,
	})
	if elements.NodeIds == nil || elements.EdgeIds == nil ||
		len(elements.NodeIds) != 0 || len(elements.EdgeIds) != 0 {
		t.Fatalf("elements=%+v want two empty lists", elements)
	}
}
