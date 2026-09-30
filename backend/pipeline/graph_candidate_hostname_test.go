// in-package test: 要求先がホスト名である起点が、ホスト名のノードから候補のエッジを持つことを確かめる。
package pipeline

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// hostnameOrigin は graph fixture の起点のうち、要求先がホスト名である 1 件を返す。
func hostnameOrigin(t *testing.T, result ImportResult) matchSide {
	t.Helper()
	for _, origin := range collectCandidateSides(result).origins {
		if namesHostnameOnly(origin) {
			return origin
		}
	}
	t.Fatal("the fixture carries no origin whose request target is a host name")
	return matchSide{}
}

// candidateEdgeFrom は、起点のノードの種別が sourceKind である候補のエッジを 1 本返す。
//
// **エッジの並び順で選ばない。** graph fixture は IP の起点とホスト名の起点の 2 本の候補の
// エッジを持つ。
func candidateEdgeFrom(t *testing.T, graph Graph, sourceKind core.NodeKind) core.GraphEdge {
	t.Helper()
	subgraph := graph.Query(wholeGraphQuery())
	nodeKinds := make(map[string]core.NodeKind, len(subgraph.Nodes))
	for _, node := range subgraph.Nodes {
		nodeKinds[node.Id] = node.Kind
	}
	for _, edge := range subgraph.Edges {
		if edge.Kind == core.EdgeKindCrossSourceConnectionMatch &&
			nodeKinds[edge.SourceNodeId] == sourceKind {
			return edge
		}
	}
	t.Fatalf("the graph carries no candidate edge from a %s node", sourceKind)
	return core.GraphEdge{}
}

// hostnameCandidateMatches は、ホスト名のノードを起点にした候補のエッジの関連付けを返す。
func hostnameCandidateMatches(t *testing.T, graph Graph) []core.EdgeMatch {
	t.Helper()
	edge := candidateEdgeFrom(t, graph, core.NodeKindDomain)
	detail, found := graph.EdgeDetail(edge.Id, EdgeEvidenceFilter{})
	if !found {
		t.Fatalf("the edge %q has no detail", edge.Id)
	}
	return detailMatches(t, detail)
}

// destinationIpConditionOf は関連付けの条件から接続先 IP の条件を返す。
func destinationIpConditionOf(t *testing.T, match core.EdgeMatch) core.MatchCondition {
	t.Helper()
	for _, condition := range match.Conditions {
		if condition.ConditionKey == core.ConditionKeyDestinationIp {
			return condition
		}
	}
	t.Fatal("the match carries no destination_ip condition")
	return core.MatchCondition{}
}

// **要求先がホスト名である起点も候補のエッジを持つ。** 接続先 IP の条件を選んだ要求では、
// その条件の use が no_comparable_counterpart になり、端末と接続先 port と秒で絞る。
func TestHostnameOriginDerivesACandidateEdgeWithoutComparingTheDestinationIp(t *testing.T) {
	result := graphResult(t)
	graph := NewGraph(result, AllMatchConditions())
	origin := hostnameOrigin(t, result)
	key, buildable := core.NewRecordNodeKey(origin.record.Locator)
	if !buildable {
		t.Fatal("the origin record carries no node key")
	}
	derivation, found := graph.RelationDerivationOf(nodeIdOf(key))
	if !found || derivation.Outcome != core.RelationDerivationMatched {
		t.Fatalf("the host name origin derived %+v (found %v), want matched", derivation, found)
	}
	matches := hostnameCandidateMatches(t, graph)
	if len(matches) != 1 {
		t.Fatalf("the host name edge carries %d matches, want 1", len(matches))
	}
	match := matches[0]
	if match.OriginRef.LineNumber == nil || *match.OriginRef.LineNumber != 1 ||
		match.CandidateRef.SequenceNumber == nil || *match.CandidateRef.SequenceNumber != 16 {
		t.Errorf("the match joins %+v to %+v, want the Squid line 1 and the sequence number 16",
			match.OriginRef, match.CandidateRef)
	}
	used := make([]core.ConditionKey, 0, len(match.Conditions))
	for _, condition := range match.Conditions {
		if condition.Use == core.ConditionUseUsed {
			used = append(used, condition.ConditionKey)
		}
	}
	wantUsed := []core.ConditionKey{
		core.ConditionKeyTerminalIpAssignment, core.ConditionKeyDestinationPort,
		core.ConditionKeySecondOfTime,
	}
	if !slices.Equal(used, wantUsed) {
		t.Errorf("the match used the conditions %v, want %v", used, wantUsed)
	}
	condition := destinationIpConditionOf(t, match)
	if condition.Use != core.ConditionUseNoComparableCounterpart {
		t.Errorf("the destination_ip condition carries the use %q, want no_comparable_counterpart",
			condition.Use)
	}
	if len(condition.LeftValue) != 0 || len(condition.RightValue) != 0 {
		t.Errorf("the destination_ip condition carries the values %v and %v, want none",
			condition.LeftValue, condition.RightValue)
	}
}

// 接続先 IP の条件を選ばない要求では、ホスト名の起点の接続先 IP の条件も not_used である。
func TestHostnameOriginMarksTheUnselectedDestinationIpAsNotUsed(t *testing.T) {
	selection := MatchConditionSelection{Conditions: []SelectedMatchCondition{
		{ConditionKey: core.ConditionKeyTerminalIpAssignment},
		{ConditionKey: core.ConditionKeyDestinationPort},
		{ConditionKey: core.ConditionKeySecondOfTime},
	}}
	matches := hostnameCandidateMatches(t, NewGraph(graphResult(t), selection))
	if len(matches) == 0 {
		t.Fatal("the host name edge carries no match")
	}
	if use := destinationIpConditionOf(t, matches[0]).Use; use != core.ConditionUseNotUsed {
		t.Errorf("the destination_ip condition carries the use %q, want not_used", use)
	}
}

// 接続先 IP も要求先のホスト名も持たない起点は、destination_ip_absent に分ける。
func TestOriginWithoutAnyDestinationIsDestinationIpAbsent(t *testing.T) {
	result := graphResult(t)
	graph := NewGraph(result, AllMatchConditions())
	lookup := newCandidateLookup(
		collectCandidateSides(result), terminalAssignmentsOf(result), AllMatchConditions())
	derivation := graph.prepareOriginDerivation(result, lookup, matchSide{})
	if derivation.outcome != core.RelationDerivationDestinationIpAbsent {
		t.Errorf("the origin without a destination is %q, want destination_ip_absent",
			derivation.outcome)
	}
	// ホスト名だけを持つ起点は、この分類に入らず、候補集合の要求まで進む。
	hostname := graph.prepareOriginDerivation(result, lookup, hostnameOrigin(t, result))
	if hostname.outcome != core.RelationDerivationMatched {
		t.Errorf("the host name origin is %q before the candidate set, want matched",
			hostname.outcome)
	}
}
