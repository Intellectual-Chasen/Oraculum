// in-package test: 観測の層のエッジの配列と隣接の並びは公開する型を持たないため、直に読む。
package pipeline

import (
	"slices"
	"sync"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// adjacencyShape は、ノードごとの出る・入るエッジの番号の並びを 1 本に連ねる。
func adjacencyShape(graph Graph) []int {
	var shape []int
	for _, adjacency := range graph.adjacency {
		shape = append(shape, len(adjacency.outgoing), len(adjacency.incoming))
		shape = append(shape, adjacency.outgoing...)
		shape = append(shape, adjacency.incoming...)
	}
	return shape
}

// 同じ観測の層から 2 つの選択のグラフを同時に組んでも、観測の層は変わらない。
func TestTheCandidateLayerLeavesTheObservedLayerUntouched(t *testing.T) {
	result := graphResult(t)
	observed := NewObservedGraph(result)
	edges, shape := len(observed.edges), adjacencyShape(observed)
	edgeIds := slices.Sorted(func(yield func(string) bool) {
		for id := range observed.edgeAt {
			if !yield(id) {
				return
			}
		}
	})
	withoutTime := MatchConditionSelection{Conditions: []SelectedMatchCondition{
		{ConditionKey: core.ConditionKeyTerminalIpAssignment},
		{ConditionKey: core.ConditionKeyDestinationIp},
	}}

	built := make([]Graph, 2)
	var group sync.WaitGroup
	for index, selection := range []MatchConditionSelection{AllMatchConditions(), withoutTime} {
		group.Add(1)
		go func() {
			defer group.Done()
			built[index] = observed.WithCandidateEdges(result, selection)
		}()
	}
	group.Wait()

	if len(observed.edges) != edges || !slices.Equal(adjacencyShape(observed), shape) {
		t.Error("adding the candidate edges changed the edges or the adjacency of the observed layer")
	}
	if len(observed.edgeAt) != len(edgeIds) {
		t.Errorf("the observed layer indexes %d edges after the candidate layers, want %d",
			len(observed.edgeAt), len(edgeIds))
	}
	for index, graph := range built {
		if len(graph.nodes) != len(observed.nodes) || len(graph.adjacency) != len(graph.nodes) {
			t.Errorf("the graph %d carries %d nodes and %d adjacencies, the observed layer %d nodes",
				index, len(graph.nodes), len(graph.adjacency), len(observed.nodes))
		}
		// **ノードは候補のエッジを足しても変わらず、観測の層と同じ配列を共有する。**
		if len(graph.nodes) > 0 && &graph.nodes[0] != &observed.nodes[0] {
			t.Errorf("the graph %d copied the nodes, want the array of the observed layer", index)
		}
		if graph.observedEdgeCount != len(observed.edges) {
			t.Errorf("the graph %d counts %d observed edges, the observed layer carries %d",
				index, graph.observedEdgeCount, len(observed.edges))
		}
		// **候補のエッジは選択のグラフの表で探せ、観測の層の表に入らない。**
		for at, edge := range graph.edges[graph.observedEdgeCount:] {
			found, indexed := graph.edgeIndexOf(edge.id)
			if _, leaked := observed.edgeAt[edge.id]; !indexed ||
				found != graph.observedEdgeCount+at || leaked {
				t.Errorf("the graph %d indexes the candidate edge %q at %d (present=%t, in the "+
					"observed index=%t)", index, edge.id, found, indexed, leaked)
			}
		}
	}
	if len(built[0].edges) == built[0].observedEdgeCount {
		t.Fatal("the fixture yields no candidate edge under every condition; the check needs one")
	}
}

// 候補のエッジを持つグラフから組み直しても、観測の層まで切り詰めてから足す。
func TestTheCandidateLayerStartsFromTheObservedEdgesOnly(t *testing.T) {
	result := graphResult(t)
	once := NewGraph(result, AllMatchConditions())
	twice := once.WithCandidateEdges(result, AllMatchConditions())
	if len(twice.edges) != len(once.edges) {
		t.Errorf("adding the candidate edges twice yields %d edges, once yields %d",
			len(twice.edges), len(once.edges))
	}
	for index, edge := range twice.edges {
		if at, indexed := twice.edgeIndexOf(edge.id); !indexed || at != index {
			t.Errorf("the edge %d is indexed at %d (present=%t)", index, at, indexed)
		}
	}
}
