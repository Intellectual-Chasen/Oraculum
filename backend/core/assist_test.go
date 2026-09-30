package core_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

var assistTime = core.NewAssertionTime(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC))

const assistConversationId = "00112233445566778899aabbccddeeff"

// validation は Validate を持つ値と、通るかである。
type validation struct {
	name  string
	value interface{ Validate() error }
	valid bool
}

func checkValidations(t *testing.T, cases []validation) {
	t.Helper()
	for _, c := range cases {
		err := c.value.Validate()
		if c.valid && err != nil {
			t.Errorf("%s: Validate = %v, want nil", c.name, err)
		}
		if !c.valid && err == nil {
			t.Errorf("%s: Validate = nil, want an error", c.name)
		}
	}
}

func TestAssistPermissionRevisionValidate(t *testing.T) {
	valid := core.AssistPermissionRevision{Provider: core.AssistProviderClaude, RevisionNumber: 1,
		Action: core.AssistPermissionActionGrant, Analyst: "analyst-a", RecordedAt: assistTime}
	unknownProvider, zeroRevision, noAnalyst, controlAnalyst := valid, valid, valid, valid
	unknownProvider.Provider = "other"
	zeroRevision.RevisionNumber = 0
	noAnalyst.Analyst = ""
	controlAnalyst.Analyst = "analyst\n"
	checkValidations(t, []validation{
		{"valid", valid, true}, {"unknown provider", unknownProvider, false}, {"zero revision", zeroRevision, false},
		{"no analyst", noAnalyst, false}, {"control character", controlAnalyst, false},
	})
	revisions := []core.AssistPermissionRevision{valid}
	if !core.AssistPermitted(revisions, core.AssistProviderClaude) {
		t.Error("a grant does not permit")
	}
	revoked := valid
	revoked.RevisionNumber, revoked.Action = 2, core.AssistPermissionActionRevoke
	if core.AssistPermitted(append(revisions, revoked), core.AssistProviderClaude) {
		t.Error("a revocation after the grant still permits")
	}
	if core.AssistPermitted(nil, core.AssistProviderClaude) {
		t.Error("no revision permits")
	}
}

func TestAssistConversationIdentifiers(t *testing.T) {
	for _, value := range []string{assistConversationId, strings.Repeat("f", 32)} {
		if err := core.ValidateAssistConversationId("id", value); err != nil {
			t.Errorf("%q: %v", value, err)
		}
	}
	for _, value := range []string{"", strings.Repeat("F", 32), strings.Repeat("a", 31), strings.Repeat("g", 32)} {
		if core.ValidateAssistConversationId("id", value) == nil {
			t.Errorf("%q was accepted", value)
		}
	}
	for _, value := range []string{"turn-1", "a_b", strings.Repeat("a", 64)} {
		if err := core.ValidateAssistTurnId("turnId", value); err != nil {
			t.Errorf("%q: %v", value, err)
		}
	}
	for _, value := range []string{"", "turn 1", "turn/1", strings.Repeat("a", 65)} {
		if core.ValidateAssistTurnId("turnId", value) == nil {
			t.Errorf("%q was accepted", value)
		}
	}
}

func TestAssistShortRefValidate(t *testing.T) {
	line := int64(3)
	record := &core.AssistRecordTarget{SourceId: "src:1", Record: core.AssertionRecordRef{
		SourceContentSha256: strings.Repeat("a", 64), PositionKind: core.PositionKindLineNumber, LineNumber: &line,
	}}
	checkValidations(t, []validation{
		{"record", core.AssistShortRef{Ref: "r1", Kind: core.AssistRefKindRecord, Record: record}, true},
		{"node", core.AssistShortRef{Ref: "n1", Kind: core.AssistRefKindNode, NodeId: "n:x"}, true},
		{"edge", core.AssistShortRef{Ref: "e1", Kind: core.AssistRefKindEdge, EdgeId: "e:x"}, true},
		{"record without the record", core.AssistShortRef{Ref: "r1", Kind: core.AssistRefKindRecord}, false},
		{"node with a record", core.AssistShortRef{Ref: "n1", Kind: core.AssistRefKindNode, NodeId: "n:x",
			Record: record}, false},
		{"unknown kind", core.AssistShortRef{Ref: "x1", Kind: "file"}, false},
	})
	if got := core.AssistShortRefOf(core.AssistRefKindEdge, 7); got != "e7" {
		t.Errorf("AssistShortRefOf = %q, want e7", got)
	}
}

func TestAssistDisclosureValidate(t *testing.T) {
	valid := core.AssistDisclosure{
		Ordinal: 1, ConversationId: assistConversationId, TurnId: "turn-1", Tool: core.AssistToolRecords,
		Request: "{}", RecordRefs: []string{"r1"}, BodySha256: strings.Repeat("b", 64), BodyBytes: 10,
		Versions: core.AssistVersions{SourceSetSha256: strings.Repeat("c", 64), PermissionRevision: 1,
			MatchConditions: []core.AssistMatchCondition{{ConditionKey: core.ConditionKeyDestinationIp}}},
		RecordedAt: assistTime,
	}
	unknownTool, badDigest, emptyRef, negativeTolerance, noPermission := valid, valid, valid, valid, valid
	unknownTool.Tool = "shell"
	badDigest.BodySha256 = "B"
	emptyRef.RecordRefs = []string{""}
	negativeTolerance.Versions.MatchConditions = []core.AssistMatchCondition{
		{ConditionKey: core.ConditionKeySecondOfTime, Tolerance: -1},
	}
	noPermission.Versions.PermissionRevision = 0
	checkValidations(t, []validation{
		{"valid", valid, true}, {"unknown tool", unknownTool, false}, {"bad digest", badDigest, false},
		{"empty record reference", emptyRef, false}, {"negative tolerance", negativeTolerance, false},
		{"no permission revision", noPermission, false},
	})
}

func TestAssistEventValidate(t *testing.T) {
	query := &core.SearchQuery{Depth: 1}
	originQuery := &core.SearchQuery{Depth: 2, NodeIds: []string{"node-a", "node-b"}}
	origins := []core.AssistOrigin{
		{Id: "node-a", Kind: core.NodeKindProcess, Label: "a.exe"},
		{Id: "node-b", Kind: core.NodeKindRecord, Label: "record b"},
	}
	checkValidations(t, []validation{
		{"text", core.AssistEvent{Sequence: 1, TurnId: "turn-1", Kind: core.AssistEventKindText, Text: "x"}, true},
		{"card", core.AssistEvent{Sequence: 2, TurnId: "turn-1", Kind: core.AssistEventKindSearchQueryCard,
			SearchQuery: query}, true},
		{"card without a query", core.AssistEvent{Sequence: 2, TurnId: "turn-1",
			Kind: core.AssistEventKindSearchQueryCard}, false},
		{"text with a query", core.AssistEvent{Sequence: 2, TurnId: "turn-1", Kind: core.AssistEventKindText,
			SearchQuery: query}, false},
		{"card with an invalid query", core.AssistEvent{Sequence: 2, TurnId: "turn-1",
			Kind: core.AssistEventKindSearchQueryCard, SearchQuery: &core.SearchQuery{Depth: 99}}, false},
		{"tool use", core.AssistEvent{Sequence: 2, TurnId: "turn-1", Kind: core.AssistEventKindToolUse,
			ToolName: "graph_search", ToolInput: []byte(`{"searchQuery":{}}`)}, true},
		{"tool use without a name", core.AssistEvent{Sequence: 2, TurnId: "turn-1",
			Kind: core.AssistEventKindToolUse}, false},
		{"tool use with a result", core.AssistEvent{Sequence: 2, TurnId: "turn-1", Kind: core.AssistEventKindToolUse,
			ToolName: "overview", ToolResult: "{}"}, false},
		{"tool use with a query", core.AssistEvent{Sequence: 2, TurnId: "turn-1", Kind: core.AssistEventKindToolUse,
			ToolName: "graph_search", SearchQuery: query}, false},
		{"tool result", core.AssistEvent{Sequence: 3, TurnId: "turn-1", Kind: core.AssistEventKindToolResult,
			ToolName: "graph_search", ToolUseSequence: 2, ToolResult: "{}", SearchQuery: query}, true},
		{"card with its origins", core.AssistEvent{Sequence: 2, TurnId: "turn-1",
			Kind: core.AssistEventKindSearchQueryCard, SearchQuery: originQuery, Origins: origins}, true},
		{"card with an unnamed origin", core.AssistEvent{Sequence: 2, TurnId: "turn-1",
			Kind: core.AssistEventKindSearchQueryCard, SearchQuery: originQuery, Origins: []core.AssistOrigin{
				{Id: "node-a", Kind: core.NodeKindProcess}, {Id: "node-b", Kind: core.NodeKindRecord}}}, true},
		{"card with origin nodes but no origins", core.AssistEvent{Sequence: 2, TurnId: "turn-1",
			Kind: core.AssistEventKindSearchQueryCard, SearchQuery: originQuery}, false},
		{"card with origins out of order", core.AssistEvent{Sequence: 2, TurnId: "turn-1",
			Kind: core.AssistEventKindSearchQueryCard, SearchQuery: originQuery,
			Origins: []core.AssistOrigin{origins[1], origins[0]}}, false},
		{"card with an origin of an unknown kind", core.AssistEvent{Sequence: 2, TurnId: "turn-1",
			Kind: core.AssistEventKindSearchQueryCard, SearchQuery: originQuery, Origins: []core.AssistOrigin{
				origins[0], {Id: "node-b", Kind: "unknown"}}}, false},
		{"text with origins", core.AssistEvent{Sequence: 2, TurnId: "turn-1", Kind: core.AssistEventKindText,
			Text: "x", Origins: origins}, false},
		{"failed tool result", core.AssistEvent{Sequence: 3, TurnId: "turn-1", Kind: core.AssistEventKindToolResult,
			ToolName: "overview", ToolUseSequence: 2, ToolResult: "denied", ToolFailed: true}, true},
		{"tool result without its call", core.AssistEvent{Sequence: 3, TurnId: "turn-1",
			Kind: core.AssistEventKindToolResult, ToolName: "overview"}, false},
		{"tool result before its call", core.AssistEvent{Sequence: 3, TurnId: "turn-1",
			Kind: core.AssistEventKindToolResult, ToolName: "overview", ToolUseSequence: 3}, false},
		{"tool result with an input", core.AssistEvent{Sequence: 3, TurnId: "turn-1",
			Kind: core.AssistEventKindToolResult, ToolName: "overview", ToolUseSequence: 2,
			ToolInput: []byte(`{}`)}, false},
		{"text with a tool name", core.AssistEvent{Sequence: 2, TurnId: "turn-1", Kind: core.AssistEventKindText,
			Text: "x", ToolName: "overview"}, false},
		{"no sequence", core.AssistEvent{TurnId: "turn-1", Kind: core.AssistEventKindTurnEnd}, false},
		{"unknown kind", core.AssistEvent{Sequence: 1, TurnId: "turn-1", Kind: "image"}, false},
	})
}
