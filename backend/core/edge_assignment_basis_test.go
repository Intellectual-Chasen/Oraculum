package core_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// assignmentBasis は検査を通る成立の根拠を組む。
func assignmentBasis(t *testing.T) core.EdgeAssignmentBasis {
	t.Helper()
	outside := false
	return core.EdgeAssignmentBasis{
		ClientIp: "192.0.2.1",
		SourceId: "source-a",
		Conditions: []core.MatchCondition{{
			ConditionKey:           core.ConditionKeyTerminalIpAssignment,
			Use:                    core.ConditionUseUsed,
			LeftValue:              []core.RecordField{textField(t, "srcIP", core.SemanticKeyConnectionSourceAddress, "192.0.2.1")},
			RightValue:             []core.RecordField{textField(t, "clientTerminal", core.SemanticKeyTerminalId, "T1")},
			AssignmentValidRange:   &core.TimeRange{From: markIIEventTime(t), To: markIIEventTime(t)},
			OutsideAssignmentRange: &outside,
		}},
		Assumptions:         []core.MatchAssumption{},
		ClockDependencyNote: "割当の期間とレコードの時刻の比較が収集元の時計に依拠する",
	}
}

func TestEdgeAssignmentBasisAcceptsTheAssignmentCondition(t *testing.T) {
	if err := assignmentBasis(t).Validate(); err != nil {
		t.Fatalf("a complete assignment basis was rejected: %v", err)
	}
}

// **割当の条件を持たない根拠を受け取らない。** 条件が 1 件もない根拠と、別の条件だけの
// 根拠の両方を拒否する。
func TestEdgeAssignmentBasisRejectsTheBasisWithoutTheAssignmentCondition(t *testing.T) {
	empty := assignmentBasis(t)
	empty.Conditions = nil
	if err := empty.Validate(); err == nil {
		t.Error("a basis without conditions was accepted")
	}

	other := assignmentBasis(t)
	other.Conditions = []core.MatchCondition{{
		ConditionKey: core.ConditionKeyDestinationIp,
		Use:          core.ConditionUseNotUsed,
	}}
	if err := other.Validate(); err == nil {
		t.Error("a basis carrying no terminal_ip_assignment condition was accepted")
	}
}

func TestEdgeAssignmentBasisRequiresTheAddressAndTheClockNote(t *testing.T) {
	withoutIp := assignmentBasis(t)
	withoutIp.ClientIp = ""
	if err := withoutIp.Validate(); err == nil {
		t.Error("a basis without the client address was accepted")
	}

	withoutNote := assignmentBasis(t)
	withoutNote.ClockDependencyNote = ""
	if err := withoutNote.Validate(); err == nil {
		t.Error("a basis without the clock dependency note was accepted")
	}
}

// 端末の外部識別子から組む識別鍵は、レコードから組む鍵と同じ形と値になる。
func TestTerminalNodeKeyMatchesTheKeyBuiltFromARecord(t *testing.T) {
	built, ok := core.TerminalNodeKey("T1")
	if !ok {
		t.Fatal("a terminal id built no node key")
	}
	if built.Kind != core.NodeKindTerminal || built.Form != core.NodeKeyFormTerminalId {
		t.Errorf("the key is %s/%s, want terminal/terminal_id", built.Kind, built.Form)
	}
	if len(built.Values) != 1 || built.Values[0].Value != "T1" ||
		built.Values[0].Semantic != core.SemanticKeyTerminalId {
		t.Errorf("the key carries the identity %+v, want terminal.id T1", built.Values)
	}
	graph := core.NewRecordGraph([]core.RecordField{
		textField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
	})
	if len(graph.Nodes) != 1 {
		t.Fatalf("the record graph carries %d nodes, want 1", len(graph.Nodes))
	}
	if graph.Nodes[0].Form != built.Form ||
		graph.Nodes[0].Values[0] != built.Values[0] {
		t.Errorf("the record built %+v, want %+v", graph.Nodes[0], built)
	}
	if _, empty := core.TerminalNodeKey(""); empty {
		t.Error("an empty terminal id built a node key")
	}
}
