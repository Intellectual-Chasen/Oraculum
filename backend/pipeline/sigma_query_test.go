// in-package test: Sigma の一致を検索の条件でフィルタし、一致のノードとエッジを求める。
package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func TestSigmaMatchesInFiltersByTheTextConditions(t *testing.T) {
	graph := orderTestGraph(t,
		orderTestRecord(t, 1, "{P1}", `C:\a.txt`),
		orderTestRecord(t, 2, "{P2}", `C:\b.txt`),
		orderTestRecord(t, 3, "{P2}", `C:\c.txt`))
	sigma := sigmaTestEvaluation(graph,
		sigmaTestRule{level: "high", records: []int{0, 1}},
		sigmaTestRule{level: "low", records: []int{0}})
	outside := int64(99)
	sigma.Matches = append(sigma.Matches, SigmaRuleMatch{
		RulePath: sigma.Rules[0].Path,
		Record:   core.RecordLocator{SourceFileName: "other.log", PositionKind: core.PositionKindLineNumber, LineNumber: &outside},
	})

	all := graph.SigmaMatchesIn(GraphQuery{}, sigma)
	if len(all.Matches) != 4 || len(all.Rules) != 2 || all.OutsideGraphMatchCount != 0 {
		t.Fatalf("without conditions = %d matches, %d rules, %d outside; want every match",
			len(all.Matches), len(all.Rules), all.OutsideGraphMatchCount)
	}
	if all.Matches[len(all.Matches)-1].RecordNode != nil {
		t.Error("the match outside the graph has a record node")
	}

	filtered := graph.SigmaMatchesIn(GraphQuery{ValueContains: []string{"b.txt"}}, sigma)
	if filtered.OutsideGraphMatchCount != 1 {
		t.Errorf("outside graph = %d, want 1", filtered.OutsideGraphMatchCount)
	}
	if len(filtered.Matches) != 1 || filtered.Matches[0].Record != graph.records[1].locator {
		t.Fatalf("matches = %+v, want the 1 match of the record that contains b.txt", filtered.Matches)
	}
	recordNode := filtered.Matches[0].RecordNode
	if recordNode == nil || recordNode.Id != graph.nodes[graph.records[1].recordNode].id {
		t.Errorf("record node = %+v, want the node of the matched record", recordNode)
	}
	if len(filtered.Rules) != 1 || filtered.Rules[0].Path != sigma.Rules[0].Path || filtered.Rules[0].MatchCount != 1 {
		t.Fatalf("rules = %+v, want the high rule with 1 match and no low rule", filtered.Rules)
	}
	want := graph.RecordGraphElements(graph.records[1].locator)
	if got := filtered.Rules[0].Graph; !slices.Equal(got.NodeIds, want.NodeIds) || !slices.Equal(got.EdgeIds, want.EdgeIds) ||
		len(got.EdgeIds) == 0 {
		t.Errorf("rule graph = %+v, want the elements of the matched record %+v", got, want)
	}
}
