package investigationdb_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/investigationdb"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// proposalConversation は会話の識別子である。
const proposalConversation = "00112233445566778899aabbccddeeff"

func sampleProposals() []investigationdb.StoredAssistProposal {
	byLine := core.AssertionRecordRef{SourceContentSha256: shaA, PositionKind: core.PositionKindLineNumber,
		LineNumber: intPointer(12)}
	bySequence := core.AssertionRecordRef{SourceContentSha256: shaB, PositionKind: core.PositionKindSequenceNumber,
		SequenceNumber: intPointer(7)}
	return []investigationdb.StoredAssistProposal{
		{Ordinal: 1, Proposal: core.AssistProposal{
			Id: "ap:edge", Target: core.AssertionTarget{Kind: core.AssertionTargetKindEdge, Edge: &core.AssertionEdgeRef{
				Kind: core.EdgeKindProcessCommunication, SourceNodeId: "n:process:1", TargetNodeId: "n:ip:2"}},
			Note: "同じ時刻に接続している", RecordRefs: []core.AssertionRecordRef{byLine, bySequence},
			ConversationId: proposalConversation, TurnId: "turn-1", Provider: core.AssistProviderClaude,
			Model: "model-a", AddsRelation: true,
			MatchConditions: []core.AssistMatchCondition{
				{ConditionKey: core.ConditionKeyDestinationIp},
				{ConditionKey: core.ConditionKeyDestinationPort, Tolerance: 3},
			},
			State: core.AssistProposalStateProposed, CreatedAt: recordedAt(5),
		}},
		{Ordinal: 2, Proposal: core.AssistProposal{
			Id: "ap:record", Target: core.AssertionTarget{Kind: core.AssertionTargetKindRecord, Record: &bySequence},
			Note: "不審な起動", RecordRefs: []core.AssertionRecordRef{bySequence},
			ConversationId: proposalConversation, TurnId: "turn-2", Provider: core.AssistProviderClaude,
			State: core.AssistProposalStateProposed, CreatedAt: recordedAt(6),
		}},
	}
}

// columnsOf は表の列の名前を定義の順に返す。
func columnsOf(t *testing.T, path, table string) []string {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

// AI 提案の表を持たない調査の file を開き、提案を書き、採用と却下を記録し、開き直して同じ値で読む。
// 採用で作った所見は元の提案の識別子を持ち、既存の所見の表の列は変わらない。
func TestAssistProposalsAreRecordedDecidedAndReadBack(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "investigation")
	path := filepath.Join(dir, investigationdb.FileName)
	db := create(t, dir)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	assertionColumns := columnsOf(t, path, "assertion")
	db, contents, err := investigationdb.Open(ctx, dir)
	if err != nil || contents.AssistProposals != nil {
		t.Fatalf("proposals=%+v err=%v, want a file without the proposal tables to open empty",
			contents.AssistProposals, err)
	}
	stored := sampleProposals()
	for _, item := range stored {
		if err := db.InsertAssistProposal(ctx, item.Proposal, item.Ordinal); err != nil {
			t.Fatal(err)
		}
	}
	adopted := stored[0].Proposal
	adopted.State = core.AssistProposalStateAdopted
	adopted.Decision = &core.AssistProposalDecision{Analyst: "analyst-a", DecidedAt: recordedAt(7), AssertionId: "as:adopted"}
	assertion := core.Assertion{
		Id: "as:adopted", Target: adopted.Target, State: core.AssertionStateActive, Author: "analyst-a",
		RecordedAt: recordedAt(7), Basis: core.AssertionBasis{Note: "分析者が直した記述", RecordRefs: adopted.RecordRefs},
		AddsRelation: true, ProposalId: adopted.Id, RevisionNumber: core.FirstAssertionRevisionNumber,
		History: []core.AssertionRevision{},
	}
	if err := db.AdoptAssistProposal(ctx, adopted, assertion, 1); err != nil {
		t.Fatal(err)
	}
	rejected := stored[1].Proposal
	rejected.State = core.AssistProposalStateRejected
	rejected.Decision = &core.AssistProposalDecision{Analyst: "analyst-b", DecidedAt: recordedAt(8), Reason: "別の端末"}
	if err := db.RejectAssistProposal(ctx, rejected); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if got := columnsOf(t, path, "assertion"); !slices.Equal(got, assertionColumns) {
		t.Errorf("assertion columns = %v, want the columns before the proposals %v", got, assertionColumns)
	}
	db, contents, err = investigationdb.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	want := []investigationdb.StoredAssistProposal{{Proposal: adopted, Ordinal: 1}, {Proposal: rejected, Ordinal: 2}}
	if !reflect.DeepEqual(contents.AssistProposals, want) {
		t.Errorf("proposals=%s\nwant=%s", jsonOf(t, contents.AssistProposals), jsonOf(t, want))
	}
	if want := []investigationdb.StoredAssertion{{Assertion: assertion, Ordinal: 1}}; !reflect.DeepEqual(contents.Assertions, want) {
		t.Errorf("assertions=%s\nwant=%s", jsonOf(t, contents.Assertions), jsonOf(t, want))
	}
}

// 採否を決めた提案の 2 件目の採用と却下は ErrAssistProposalDecided になり、所見も対応も書かない。
func TestDecidedAssistProposalRefusesASecondDecision(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "investigation")
	db := create(t, dir)
	defer db.Close()
	proposal := sampleProposals()[1].Proposal
	if err := db.InsertAssistProposal(ctx, proposal, 1); err != nil {
		t.Fatal(err)
	}
	adoption := func(assertionId, analyst string) (core.AssistProposal, core.Assertion) {
		adopted := proposal
		adopted.State = core.AssistProposalStateAdopted
		adopted.Decision = &core.AssistProposalDecision{Analyst: analyst, DecidedAt: recordedAt(7), AssertionId: assertionId}
		return adopted, core.Assertion{
			Id: assertionId, Target: proposal.Target, State: core.AssertionStateActive, Author: analyst,
			RecordedAt: recordedAt(7), Basis: core.AssertionBasis{Note: proposal.Note, RecordRefs: proposal.RecordRefs},
			ProposalId: proposal.Id, RevisionNumber: core.FirstAssertionRevisionNumber, History: []core.AssertionRevision{},
		}
	}
	first, firstAssertion := adoption("as:first", "analyst-a")
	if err := db.AdoptAssistProposal(ctx, first, firstAssertion, 1); err != nil {
		t.Fatal(err)
	}
	second, secondAssertion := adoption("as:second", "analyst-b")
	if err := db.AdoptAssistProposal(ctx, second, secondAssertion, 2); !errors.Is(err, investigationdb.ErrAssistProposalDecided) {
		t.Fatalf("second adoption = %v, want ErrAssistProposalDecided", err)
	}
	rejected := proposal
	rejected.State = core.AssistProposalStateRejected
	rejected.Decision = &core.AssistProposalDecision{Analyst: "analyst-b", DecidedAt: recordedAt(8)}
	if err := db.RejectAssistProposal(ctx, rejected); !errors.Is(err, investigationdb.ErrAssistProposalDecided) {
		t.Fatalf("rejection after the adoption = %v, want ErrAssistProposalDecided", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, contents, err := investigationdb.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if len(contents.Assertions) != 1 || contents.Assertions[0].Assertion.Id != "as:first" {
		t.Errorf("assertions=%s, want the first adoption alone", jsonOf(t, contents.Assertions))
	}
	if len(contents.AssistProposals) != 1 || !reflect.DeepEqual(contents.AssistProposals[0].Proposal, first) {
		t.Errorf("proposals=%s, want the first adoption", jsonOf(t, contents.AssistProposals))
	}
}

// 理由を持たない却下は、開き直した後も理由を持たない却下として読む。
func TestRejectionWithoutAReasonReadsBackWithoutIt(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "investigation")
	db := create(t, dir)
	proposal := sampleProposals()[1].Proposal
	if err := db.InsertAssistProposal(ctx, proposal, 1); err != nil {
		t.Fatal(err)
	}
	rejected := proposal
	rejected.State = core.AssistProposalStateRejected
	rejected.Decision = &core.AssistProposalDecision{Analyst: "analyst-b", DecidedAt: recordedAt(8)}
	if err := db.RejectAssistProposal(ctx, rejected); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, contents, err := investigationdb.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	want := []investigationdb.StoredAssistProposal{{Proposal: rejected, Ordinal: 1}}
	if !reflect.DeepEqual(contents.AssistProposals, want) {
		t.Errorf("proposals=%s\nwant=%s", jsonOf(t, contents.AssistProposals), jsonOf(t, want))
	}
}

// 採用の行が所見との対応を持たない file は、採用で作った所見を指せないため開かない。
func TestOpenRefusesAnAdoptedProposalWithoutItsAssertion(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "investigation")
	path := filepath.Join(dir, investigationdb.FileName)
	db := create(t, dir)
	if err := db.InsertAssistProposal(ctx, sampleProposals()[1].Proposal, 1); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	execRaw(t, path, `UPDATE assist_proposal SET state = 'adopted', decided_by = 'analyst-a',
		decided_at = '2030-01-02T03:07:00.000Z'`)
	_, _, err := investigationdb.Open(ctx, dir)
	if !errors.Is(err, core.ErrMissingRequiredItem) || !strings.Contains(err.Error(), "assertionId") {
		t.Fatalf("Open() = %v, want the adoption refused for its missing assertion identifier", err)
	}
}
