// in-package test: 調査の保存先へ書く関数の組 (非公開の journal) を差し替えて、書けなかった採否を確かめる。
package pipeline

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func proposalDraft() AssistProposalDraft {
	line := int64(3)
	return AssistProposalDraft{
		Target: core.AssertionTarget{Kind: core.AssertionTargetKindNode, NodeId: "n:process:a"},
		Note:   "LLM が挙げた候補",
		RecordRefs: []core.AssertionRecordRef{{
			SourceContentSha256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			PositionKind:        core.PositionKindLineNumber, LineNumber: &line,
		}},
		ConversationId: "0123456789abcdef0123456789abcdef", TurnId: "turn-1", Provider: core.AssistProviderClaude,
	}
}

// recordingJournal は、書くように渡された提案と所見を記録し、failure が nil でなければ失敗を返す。
type recordingJournal struct {
	failure         error
	created         []core.AssistProposal
	createdOrdinals []int64
	adopted         []core.AssistProposal
	assertion       []core.Assertion
	ordinals        []int64
	rejected        []core.AssistProposal
}

func (j *recordingJournal) journal() assistProposalJournal {
	return assistProposalJournal{
		created: func(proposal core.AssistProposal, ordinal int64) error {
			j.created = append(j.created, proposal)
			j.createdOrdinals = append(j.createdOrdinals, ordinal)
			return j.failure
		},
		adopted: func(proposal core.AssistProposal, assertion core.Assertion, ordinal int64) error {
			j.adopted, j.assertion = append(j.adopted, proposal), append(j.assertion, assertion)
			j.ordinals = append(j.ordinals, ordinal)
			return j.failure
		},
		rejected: func(proposal core.AssistProposal) error {
			j.rejected = append(j.rejected, proposal)
			return j.failure
		},
	}
}

func journaledProposals(t *testing.T, journal *recordingJournal) (*MemoryAssistProposalStore, *MemoryAssertionStore) {
	t.Helper()
	clock := &steppingClock{}
	assertions := NewMemoryAssertionStore(clock)
	store, err := newJournaledAssistProposalStore(clock, assertions, journal.journal(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return store, assertions
}

// 採用は提案の状態の変更と所見を 1 回の書き込みで渡し、書けた後に両方を確定する。
func TestAdoptionWritesTheProposalAndTheAssertionTogether(t *testing.T) {
	journal := &recordingJournal{}
	store, assertions := journaledProposals(t, journal)
	proposal, err := store.Create(proposalDraft())
	if err != nil {
		t.Fatal(err)
	}
	adopted, assertion, err := store.Adopt(proposal.Id, AssistProposalAdoption{Analyst: "analyst-a", Note: "直した記述"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(journal.created, []core.AssistProposal{proposal}) ||
		!reflect.DeepEqual(journal.createdOrdinals, []int64{1}) {
		t.Errorf("created = %+v at %v, want the proposal at the ordinal 1", journal.created, journal.createdOrdinals)
	}
	if !reflect.DeepEqual(journal.adopted, []core.AssistProposal{adopted}) ||
		!reflect.DeepEqual(journal.assertion, []core.Assertion{assertion}) || !reflect.DeepEqual(journal.ordinals, []int64{1}) {
		t.Errorf("journal = %+v, want the adopted proposal and its assertion in one write", journal)
	}
	if listed := assertions.List(); !reflect.DeepEqual(listed, []core.Assertion{assertion}) {
		t.Errorf("assertions = %+v, want the adopted assertion", listed)
	}
	if found, _ := store.Find(proposal.Id); !reflect.DeepEqual(found, adopted) {
		t.Errorf("proposal = %+v, want %+v", found, adopted)
	}
}

// 書けなかった採用と却下は、提案を提案中のまま残し、所見も通番も確定しない。
func TestFailedWriteLeavesTheProposalProposed(t *testing.T) {
	journal := &recordingJournal{}
	store, assertions := journaledProposals(t, journal)
	proposal, err := store.Create(proposalDraft())
	if err != nil {
		t.Fatal(err)
	}
	journal.failure = errors.New("disk full")
	if _, _, err := store.Adopt(proposal.Id, AssistProposalAdoption{Analyst: "analyst-a", Note: "記述"}); !errors.Is(err, ErrAssertionStoreFailure) {
		t.Fatalf("Adopt() = %v, want ErrAssertionStoreFailure", err)
	}
	if _, err := store.Reject(proposal.Id, "analyst-a", ""); !errors.Is(err, ErrAssertionStoreFailure) {
		t.Fatalf("Reject() = %v, want ErrAssertionStoreFailure", err)
	}
	if _, err := store.Create(proposalDraft()); !errors.Is(err, ErrAssertionStoreFailure) {
		t.Fatalf("Create() = %v, want ErrAssertionStoreFailure", err)
	}
	if found, _ := store.Find(proposal.Id); !reflect.DeepEqual(found, proposal) {
		t.Errorf("proposal = %+v, want it unchanged %+v", found, proposal)
	}
	if len(assertions.List()) != 0 || len(store.List()) != 1 {
		t.Errorf("assertions = %+v, proposals = %+v, want nothing settled", assertions.List(), store.List())
	}
	journal.failure = nil
	_, assertion, err := store.Adopt(proposal.Id, AssistProposalAdoption{Analyst: "analyst-a", Note: "記述"})
	if err != nil {
		t.Fatal(err)
	}
	if last := journal.ordinals[len(journal.ordinals)-1]; last != 1 {
		t.Errorf("assertion ordinal = %d, want the ordinal the failed write did not use", last)
	}
	if assertion.ProposalId != proposal.Id {
		t.Errorf("assertion = %+v, want the proposal identifier", assertion)
	}
}

// 検査を通らない採用 (空白だけの記述) は要求の不備であり、保存先の失敗に数えず、何も書かない。
func TestInvalidAdoptionIsARequestFault(t *testing.T) {
	journal := &recordingJournal{}
	store, _ := journaledProposals(t, journal)
	proposal, err := store.Create(proposalDraft())
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.Adopt(proposal.Id, AssistProposalAdoption{Analyst: "analyst-a", Note: " "})
	if !errors.Is(err, core.ErrInvalid) || errors.Is(err, ErrAssertionStoreFailure) {
		t.Fatalf("Adopt() = %v, want a request fault", err)
	}
	if len(journal.adopted) != 0 {
		t.Errorf("journal = %+v, want nothing written", journal.adopted)
	}
}

// 調査を持たない起動の保存先は、提案の作成と採否を ErrAssistUnavailable で退ける。
func TestUnavailableStoreRefusesEveryWrite(t *testing.T) {
	store := NewUnavailableAssistProposalStore()
	if store.Available() || len(store.List()) != 0 {
		t.Fatalf("store = %+v, want an unavailable empty store", store)
	}
	if _, err := store.Create(proposalDraft()); !errors.Is(err, ErrAssistUnavailable) {
		t.Errorf("Create() = %v, want ErrAssistUnavailable", err)
	}
	if _, _, err := store.Adopt("ap:1", AssistProposalAdoption{Analyst: "analyst-a", Note: "記述"}); !errors.Is(err, ErrAssistUnavailable) {
		t.Errorf("Adopt() = %v, want ErrAssistUnavailable", err)
	}
	if _, err := store.Reject("ap:1", "analyst-a", ""); !errors.Is(err, ErrAssistUnavailable) {
		t.Errorf("Reject() = %v, want ErrAssistUnavailable", err)
	}
}

// 読み戻した提案の次の提案は、読み戻した通番の次の通番で別の識別子になる。
func TestRestoredProposalsContinueTheOrdinal(t *testing.T) {
	journal := &recordingJournal{}
	first, _ := journaledProposals(t, journal)
	proposal, err := first.Create(proposalDraft())
	if err != nil {
		t.Fatal(err)
	}
	clock := &steppingClock{}
	restored, err := newJournaledAssistProposalStore(clock, NewMemoryAssertionStore(clock), journal.journal(),
		[]recordedAssistProposal{{proposal: proposal, ordinal: 1}})
	if err != nil {
		t.Fatal(err)
	}
	next, err := restored.Create(proposalDraft())
	if err != nil {
		t.Fatal(err)
	}
	if next.Id == proposal.Id || len(restored.List()) != 2 {
		t.Errorf("next = %q, restored = %+v, want a new identifier after the restored one", next.Id, restored.List())
	}
	if !reflect.DeepEqual(journal.createdOrdinals, []int64{1, 2}) {
		t.Errorf("created ordinals = %v, want the ordinal after the restored one", journal.createdOrdinals)
	}
}
