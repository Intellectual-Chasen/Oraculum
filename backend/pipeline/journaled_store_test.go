// in-package test: 保存先へ書いてから確定する store と、記録した通番の発行器は非公開であり、直に組んで読む。
package pipeline

import (
	"errors"
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

var errTestJournalRefuses = errors.New("the test journal refuses the write")

func journalPointer(value string) *string { return &value }

func journalTimestamp(t *testing.T, normalized string) core.Timestamp {
	t.Helper()
	value, err := core.NewTimestamp(core.Timestamp{
		RawText: journalPointer(normalized), Normalized: journalPointer(normalized),
		NormalizedForm: core.NormalizedFormRFC3339Absolute, Precision: core.PrecisionSecond,
		OffsetState: core.OffsetStateInValue, OffsetText: journalPointer("Z"), Clock: core.ClockTerminalLocal,
		Meaning: core.MeaningEvent, ValueState: core.ValueStatePresent,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

// journalAssignment は分析者が与えた割当 1 件の値である。
func journalAssignment(t *testing.T) core.TerminalAssignment {
	t.Helper()
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	line := int64(3)
	return core.TerminalAssignment{
		ClientIp: "192.0.2.10", TerminalId: "terminal-1", TerminalHostname: "pc01.example.test",
		SourceId: "src:1", SourceContentSha256: sha,
		AssignmentValidRange: core.TimeRange{From: journalTimestamp(t, "2030-01-02T03:00:00Z"),
			To: journalTimestamp(t, "2030-01-02T04:00:00Z")},
		Origin: core.TerminalAssignmentOriginAnalystSupplied, Derivation: "DHCP の記録から導いた",
		BasisRecordRefs: []core.AssertionRecordRef{{SourceContentSha256: sha,
			PositionKind: core.PositionKindLineNumber, LineNumber: &line}},
		Author: "analyst-a",
	}
}

func journalDraft(nodeId string) AssertionDraft {
	return AssertionDraft{
		Target: core.AssertionTarget{Kind: core.AssertionTargetKindNode, NodeId: nodeId},
		Author: "analyst-a", Basis: core.AssertionBasis{Note: "所見"},
	}
}

// 保存先へ書けた所見だけを確定し、書いた所見と通番は確定した所見と同じである。
func TestAJournaledStoreCommitsOnlyWhatItWrote(t *testing.T) {
	var written []recordedAssertion
	refuse := false
	store, err := newJournaledAssertionStore(&steppingClock{}, assertionJournal{
		created: func(assertion core.Assertion, ordinal int64) error {
			if refuse {
				return errTestJournalRefuses
			}
			written = append(written, recordedAssertion{assertion: assertion, ordinal: ordinal})
			return nil
		},
		revised: func(core.Assertion) error { return errTestJournalRefuses },
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	refuse = true
	if _, err := store.Create(journalDraft("n:terminal:1")); !errors.Is(err, ErrAssertionStoreFailure) ||
		!errors.Is(err, errTestJournalRefuses) {
		t.Fatalf("the refused create returned %v", err)
	}
	if listed := store.List(); len(listed) != 0 {
		t.Fatalf("the refused assertion was committed: %+v", listed)
	}
	refuse = false
	created, err := store.Create(journalDraft("n:terminal:1"))
	if err != nil {
		t.Fatal(err)
	}
	// 退けた記録に通番を使わない。保存先を持たない store と同じ識別子になる。
	plain, err := NewMemoryAssertionStore(&steppingClock{}).Create(journalDraft("n:terminal:1"))
	if err != nil {
		t.Fatal(err)
	}
	if created.Id != plain.Id {
		t.Errorf("the committed assertion is %q, the store without a journal gives %q", created.Id, plain.Id)
	}
	if len(written) != 1 || written[0].assertion.Id != created.Id || written[0].ordinal != 1 {
		t.Errorf("the journal received %+v", written)
	}
	if _, err := store.Revise(created.Id, AssertionRevisionDraft{State: core.AssertionStateWithdrawn,
		Author: "analyst-b", Basis: core.AssertionBasis{Note: "取り下げ"},
		BaseRevision: created.RevisionNumber}); !errors.Is(err, ErrAssertionStoreFailure) {
		t.Fatalf("the refused revision returned %v", err)
	}
	if found, _ := store.Find(created.Id); found.RevisionNumber != core.FirstAssertionRevisionNumber {
		t.Errorf("the refused revision was committed: revision %d", found.RevisionNumber)
	}
}

// 読み戻した所見を記録した順に持ち、次の通番は読み戻した通番の最大の次にする。
func TestARestoredStoreContinuesTheOrdinals(t *testing.T) {
	first, err := NewMemoryAssertionStore(&steppingClock{}).Create(journalDraft("n:terminal:1"))
	if err != nil {
		t.Fatal(err)
	}
	var ordinals []int64
	store, err := newJournaledAssertionStore(&steppingClock{}, assertionJournal{
		created: func(_ core.Assertion, ordinal int64) error {
			ordinals = append(ordinals, ordinal)
			return nil
		},
	}, []recordedAssertion{{assertion: first, ordinal: 4}})
	if err != nil {
		t.Fatal(err)
	}
	next, err := store.Create(journalDraft("n:terminal:1"))
	if err != nil {
		t.Fatal(err)
	}
	if next.Id == first.Id || !slices.Equal(ordinals, []int64{5}) {
		t.Errorf("the next assertion is %q with ordinals %v after the restored %q", next.Id, ordinals, first.Id)
	}
	if listed := store.List(); len(listed) != 2 || listed[0].Id != first.Id || listed[1].Id != next.Id {
		t.Errorf("the store lists %+v", listed)
	}
	if _, err := newJournaledAssertionStore(&steppingClock{}, assertionJournal{},
		[]recordedAssertion{{assertion: first, ordinal: 1}, {assertion: first, ordinal: 2}}); err == nil {
		t.Error("a store restored the same assertion twice")
	}
}

// 保存先へ書けた割当だけを確定し、読み戻した割当の件数を割当の更新回数にする。
func TestAJournaledAssignmentStoreCommitsOnlyWhatItWrote(t *testing.T) {
	assignment := journalAssignment(t)
	var written []core.TerminalAssignment
	refuse := true
	store, err := newJournaledTerminalAssignmentStore(func(value core.TerminalAssignment) error {
		if refuse {
			return errTestJournalRefuses
		}
		written = append(written, value)
		return nil
	}, []core.TerminalAssignment{assignment})
	if err != nil {
		t.Fatal(err)
	}
	if store.Revision() != 1 {
		t.Fatalf("the restored store starts at the revision %d, want the restored count", store.Revision())
	}
	draft := TerminalAssignmentDraft{
		ClientIp: "192.0.2.20", TerminalId: assignment.TerminalId, TerminalHostname: assignment.TerminalHostname,
		SourceId: assignment.SourceId, SourceContentSha256: assignment.SourceContentSha256,
		AssignmentValidRange: assignment.AssignmentValidRange, Derivation: assignment.Derivation,
		BasisRecordRefs: assignment.BasisRecordRefs, Author: assignment.Author,
	}
	if _, err := store.Create(draft); !errors.Is(err, errTestJournalRefuses) ||
		errors.Is(err, ErrTerminalAssignmentNotStorable) {
		t.Fatalf("the refused create returned %v, want the journal failure and not a request failure", err)
	}
	if listed, revision := store.List(); len(listed) != 1 || revision != 1 {
		t.Fatalf("the refused assignment changed the store: %d assignments at revision %d", len(listed), revision)
	}
	refuse = false
	created, err := store.Create(draft)
	if err != nil {
		t.Fatal(err)
	}
	if listed, revision := store.List(); len(listed) != 2 || revision != 2 || listed[1].ClientIp != created.ClientIp {
		t.Errorf("the store lists %d assignments at revision %d", len(listed), revision)
	}
	if len(written) != 1 || written[0].ClientIp != draft.ClientIp {
		t.Errorf("the journal received %+v", written)
	}
	collector := assignment
	collector.Origin = core.TerminalAssignmentOriginObservedInSource
	if _, err := newJournaledTerminalAssignmentStore(nil, []core.TerminalAssignment{collector}); err == nil {
		t.Error("a store restored an assignment the analyst did not supply")
	}
}

// 記録した収集元には記録した通番を記録した順に返し、新しい収集元には同じ組の最大の次を返す。
func TestRecordedOrdinalsReplayThenContinue(t *testing.T) {
	ordinals := newRecordedOrdinals([]recordedOrdinal{
		{originPath: "a.log", contentSha256: "x", ordinal: 2},
		{originPath: "b.log", contentSha256: "y", ordinal: 1},
		{originPath: "a.log", contentSha256: "x", ordinal: 5},
	})
	calls := [][2]string{{"a.log", "x"}, {"b.log", "y"}, {"a.log", "x"}, {"a.log", "x"}, {"c.log", "z"}}
	var got []int64
	for _, call := range calls {
		ordinal, err := ordinals.Next(call[0], call[1])
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, ordinal)
	}
	want := []int64{2, 1, 5, 6, 1}
	if !slices.Equal(got, want) || !slices.Equal(ordinals.issuedOrdinals(), want) {
		t.Errorf("the ordinals are %v and issued %v, want %v", got, ordinals.issuedOrdinals(), want)
	}
}
