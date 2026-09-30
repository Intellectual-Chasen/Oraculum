package core_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 関係の状態の軸の値は、関係の状態ごとに 1 つである。
func TestInfluenceBasisOfState(t *testing.T) {
	for state, want := range map[core.RelationState]core.InfluenceBasis{
		core.RelationStateObserved:       core.InfluenceBasisObserved,
		core.RelationStateCandidate:      core.InfluenceBasisCandidate,
		core.RelationStateUncertainChain: core.InfluenceBasisUncertainChain,
	} {
		if got := core.InfluenceBasisOfState(state); got != want {
			t.Errorf("InfluenceBasisOfState(%s) = %s, want %s", state, got, want)
		}
	}
}

// 列挙の外の値と、必須の項目を欠いた影響のエッジと要素を退ける。
func TestInfluenceItemsRejectUnknownValues(t *testing.T) {
	if core.InfluenceBasis("x").IsKnown() || core.InfluenceTimeBasis("x").IsKnown() ||
		core.InfluencePathStop("x").IsKnown() || core.InfluenceFrontierReason("x").IsKnown() ||
		core.FlowOperation("x").IsKnown() {
		t.Error("an unknown value was accepted")
	}
	edge := core.InfluenceEdge{
		Id: "i", SourceKey: "a", TargetKey: "b", GraphEdgeId: "e", GraphEdgeKind: core.EdgeKindFileOperation,
	}
	if edge.Validate() == nil {
		t.Error("an edge without evidence was accepted")
	}
	edge.Bases = []core.InfluenceBasis{"x"}
	if edge.Validate() == nil {
		t.Error("an edge with an unknown basis was accepted")
	}
	edge.Bases, edge.TimeBases = nil, []core.InfluenceTimeBasis{"x"}
	if edge.Validate() == nil {
		t.Error("an edge with an unknown time basis was accepted")
	}
	if (core.InfluenceEdge{}).Validate() == nil || (core.InfluenceVertex{}).Validate() == nil ||
		(core.InfluenceFrontier{}).Validate() == nil || (core.InfluenceEndpoint{}).Validate() == nil ||
		(core.InfluenceEndpoint{Key: "a"}).Validate() == nil {
		t.Error("an empty item was accepted")
	}
}
