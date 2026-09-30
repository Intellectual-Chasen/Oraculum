package investigationdb_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/investigationdb"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const sampleConversationId = "00112233445566778899aabbccddeeff"

func sampleConversation() core.AssistConversation {
	return core.AssistConversation{Id: sampleConversationId, Provider: core.AssistProviderClaude,
		Model: "model-a", PermissionRevision: 1, CreatedAt: recordedAt(20)}
}

func sampleTurn() core.AssistTurn {
	return core.AssistTurn{ConversationId: sampleConversationId, TurnId: "turn-1", Case: "baseline",
		MatchConditions: []core.AssistMatchCondition{
			{ConditionKey: core.ConditionKeyDestinationIp},
			{ConditionKey: core.ConditionKeySecondOfTime, Tolerance: 5},
		}}
}

func sampleRefs() []core.AssistShortRef {
	return []core.AssistShortRef{
		{Ref: "r1", Kind: core.AssistRefKindRecord, Record: &core.AssistRecordTarget{SourceId: "src:1",
			Record: core.AssertionRecordRef{SourceContentSha256: shaA, PositionKind: core.PositionKindLineNumber,
				LineNumber: intPointer(12), ByteOffset: intPointer(340)}}},
		{Ref: "n1", Kind: core.AssistRefKindNode, NodeId: "n:process:1"},
		{Ref: "e1", Kind: core.AssistRefKindEdge, EdgeId: "e:1"},
		{Ref: "r2", Kind: core.AssistRefKindRecord, Record: &core.AssistRecordTarget{SourceId: "src:2",
			Record: core.AssertionRecordRef{SourceContentSha256: shaB, PositionKind: core.PositionKindSequenceNumber,
				SequenceNumber: intPointer(7)}}},
	}
}

func sampleDisclosure(ordinal int64, tool core.AssistTool, refs []string) core.AssistDisclosure {
	return core.AssistDisclosure{
		Ordinal: ordinal, ConversationId: sampleConversationId, TurnId: "turn-1", Tool: tool,
		Request: `{"turnId":"turn-1"}`, RecordRefs: refs, Truncated: ordinal == 2,
		BodySha256: strings.Repeat("d", 64), BodyBytes: 120,
		Versions: core.AssistVersions{ServerRevision: strings.Repeat("e", 40), ServerModified: true,
			AssignmentRevision: 2, InterpretationRevision: 3, SourceSetSha256: shaA, PermissionRevision: 1,
			MatchConditions: sampleTurn().MatchConditions},
		RecordedAt: recordedAt(21),
	}
}

// 会話、発言、短い参照、受け渡しの記録を、開き直した後に同じ値で読み戻す。
func TestAssistConversationsReadBackWhatWasWritten(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db := create(t, dir)
	if err := db.InsertAssistConversation(ctx, sampleConversation()); err != nil {
		t.Fatal(err)
	}
	turn := sampleTurn()
	refs := sampleRefs()
	disclosures := []core.AssistDisclosure{
		sampleDisclosure(1, core.AssistToolTurn, []string{}),
		sampleDisclosure(2, core.AssistToolRecords, []string{"r1", "r2"}),
	}
	if err := db.InsertAssistDisclosure(ctx, disclosures[0], &turn, refs[:3]); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertAssistDisclosure(ctx, disclosures[1], nil, refs[3:]); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, contents, err := investigationdb.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	read := contents.AssistConversations
	if !reflect.DeepEqual(read.Conversations, []core.AssistConversation{sampleConversation()}) {
		t.Errorf("conversations = %+v", read.Conversations)
	}
	if !reflect.DeepEqual(read.Turns, []core.AssistTurn{turn}) {
		t.Errorf("turns = %+v, want %+v", read.Turns, turn)
	}
	// 短い参照は種類ごとに発行した順に読み戻す。
	wantRefs := []core.AssistShortRef{refs[2], refs[1], refs[0], refs[3]}
	if !reflect.DeepEqual(read.Refs[sampleConversationId], wantRefs) {
		t.Errorf("refs = %+v, want %+v", read.Refs[sampleConversationId], wantRefs)
	}
	if !reflect.DeepEqual(read.Disclosures, disclosures) {
		t.Errorf("disclosures = %+v, want %+v", read.Disclosures, disclosures)
	}
}

// 書き込みの途中で失敗した受け渡しは、発言と短い参照も残さない。
func TestAssistDisclosureWritesNothingOnAFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db := create(t, dir)
	if err := db.InsertAssistConversation(ctx, sampleConversation()); err != nil {
		t.Fatal(err)
	}
	turn := sampleTurn()
	first := sampleDisclosure(1, core.AssistToolTurn, []string{})
	if err := db.InsertAssistDisclosure(ctx, first, &turn, sampleRefs()[:1]); err != nil {
		t.Fatal(err)
	}
	// 同じ位置の受け渡しは主キーで退けられ、同じ transaction の発言と参照も巻き戻る。
	other := turn
	other.TurnId = "turn-2"
	if err := db.InsertAssistDisclosure(ctx, first, &other, sampleRefs()[1:2]); err == nil {
		t.Fatal("recording the same position twice succeeded")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, contents, err := investigationdb.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	read := contents.AssistConversations
	if len(read.Turns) != 1 || len(read.Refs[sampleConversationId]) != 1 || len(read.Disclosures) != 1 {
		t.Errorf("read = %+v, want only the first disclosure with its turn and reference", read)
	}
}
