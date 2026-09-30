package pipeline

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"sync"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// assistProposalIdPrefix は AI 提案の識別子を、所見とノードとエッジの識別子と文字列で分ける。
const assistProposalIdPrefix = "ap:"

// ErrAssistProposalNotFound は、識別子に該当する AI 提案が保存先に無いことを表す。
var ErrAssistProposalNotFound = errors.New("assist proposal store: the identifier matches no proposal")

// ErrAssistProposalDecided は、採否を決めた後の提案の採用または却下を退けたことを表す。
//
// 2 人が同じ提案を採用したときの 2 件目であり、Adopt と Reject は現在の提案を一緒に返す。
var ErrAssistProposalDecided = errors.New("assist proposal store: the proposal is already decided")

// AssistProposalDraft は新しい AI 提案 1 件の入力である。識別子・時刻・状態は保存先が決める。
type AssistProposalDraft struct {
	Target     core.AssertionTarget
	Note       string
	RecordRefs []core.AssertionRecordRef
	// ConversationId と TurnId は提案を作った会話と発言である。
	ConversationId string
	TurnId         string
	Provider       core.AssistProvider
	Model          string
	// AddsRelation は、提案がグラフに無い関係を足す提案であることである。
	AddsRelation bool
	// MatchConditions は提案を作った発言の関連付けの条件の選択である。
	MatchConditions []core.AssistMatchCondition
}

// AssistProposalAdoption は AI 提案の採用 1 件の入力である。
type AssistProposalAdoption struct {
	// Analyst は採用する分析者であり、作る所見の著者になる。
	Analyst string
	// Note は作る所見の記述である。分析者が直した記述か、提案の記述を渡す。
	Note string
	// AddsRelation は、作る所見が関係を足すことである。採用する時点のグラフで決める。
	AddsRelation bool
}

// AssistProposalStore は AI 提案を保つ保存先の port である。
//
// **提案は所見の保存先に入らない。** 所見の一覧は提案を持たず、採用で作った所見だけが元の提案の
// 識別子を持つ。
type AssistProposalStore interface {
	// Available は、この起動が AI 提案を置けるかを返す。偽の保存先は、書き込みのすべてに
	// ErrAssistUnavailable を返し、一覧は空である。
	Available() bool
	// List は記録した順に提案を返す。
	List() []core.AssistProposal
	// Find は識別子で提案を 1 件返す。ok が偽になるのは、該当する提案が無いときである。
	Find(proposalId string) (core.AssistProposal, bool)
	// Create は新しい提案を 1 件記録し、記録した提案を返す。
	Create(draft AssistProposalDraft) (core.AssistProposal, error)
	// Adopt は提案を採用に変え、分析者を著者とする所見を作る。2 つを 1 つの書き込みで確定する。
	// 提案中でない提案は ErrAssistProposalDecided と現在の提案を返す。
	Adopt(proposalId string, adoption AssistProposalAdoption) (core.AssistProposal, core.Assertion, error)
	// Reject は提案を却下に変える。reason は空でもよい。提案中でない提案は ErrAssistProposalDecided と
	// 現在の提案を返す。
	Reject(proposalId, analyst, reason string) (core.AssistProposal, error)
}

// assistProposalJournal は、AI 提案をメモリに確定する前に調査の保存先へ書く関数の組である。
//
// **保存先へ書けた提案と採否だけを確定する。**
type assistProposalJournal struct {
	// created は新しい提案と、その識別子を作った通番を書く。
	created func(proposal core.AssistProposal, ordinal int64) error
	// adopted は採用に変えた提案と、採用で作った所見とその通番を 1 つの transaction で書く。
	adopted func(proposal core.AssistProposal, assertion core.Assertion, assertionOrdinal int64) error
	// rejected は却下に変えた提案を書く。
	rejected func(proposal core.AssistProposal) error
}

// recordedAssistProposal は調査の保存先から読み戻した提案 1 件と、その識別子を作った通番である。
type recordedAssistProposal struct {
	proposal core.AssistProposal
	ordinal  int64
}

// MemoryAssistProposalStore は AssistProposalStore をメモリの上で満たす。
//
// **採用は所見の保存先の lock の中で所見を作る。** lock は提案の lock、所見の lock の順に取る。
// 所見の保存先は提案の lock を取らない。
type MemoryAssistProposalStore struct {
	clock      AssertionClock
	assertions *MemoryAssertionStore
	// available は、この起動が AI 提案を置けることである。
	available bool
	journal   assistProposalJournal
	mu        sync.Mutex
	order     []string
	byId      map[string]core.AssistProposal
	ordinal   int64
}

// NewUnavailableAssistProposalStore は、AI 提案を受け付けない保存先を返す。調査の directory を渡さない
// 起動の保存先である。
func NewUnavailableAssistProposalStore() *MemoryAssistProposalStore {
	return &MemoryAssistProposalStore{byId: map[string]core.AssistProposal{}}
}

// newJournaledAssistProposalStore は、調査の保存先から読み戻した提案を記録した順に持ち、以後の記録を
// journal へ書く保存先を返す。採用で作る所見は assertions に確定する。
func newJournaledAssistProposalStore(
	clock AssertionClock, assertions *MemoryAssertionStore, journal assistProposalJournal,
	recorded []recordedAssistProposal,
) (*MemoryAssistProposalStore, error) {
	store := &MemoryAssistProposalStore{
		clock: clock, assertions: assertions, available: true, journal: journal, byId: map[string]core.AssistProposal{},
	}
	for _, item := range recorded {
		if err := item.proposal.Validate(); err != nil {
			return nil, fmt.Errorf("restoring the assist proposal %q: %w", item.proposal.Id, err)
		}
		if _, taken := store.byId[item.proposal.Id]; taken {
			return nil, fmt.Errorf("restoring the assist proposal %q: the identifier is recorded twice: %w",
				item.proposal.Id, ErrAssertionStoreFailure)
		}
		store.byId[item.proposal.Id] = clonedProposal(item.proposal)
		store.order = append(store.order, item.proposal.Id)
		store.ordinal = max(store.ordinal, item.ordinal)
	}
	return store, nil
}

// Available は、この起動が AI 提案を置けるかを返す。
func (s *MemoryAssistProposalStore) Available() bool { return s.available }

// List は記録した順に提案を返す。
func (s *MemoryAssistProposalStore) List() []core.AssistProposal {
	s.mu.Lock()
	defer s.mu.Unlock()
	proposals := make([]core.AssistProposal, 0, len(s.order))
	for _, id := range s.order {
		proposals = append(proposals, clonedProposal(s.byId[id]))
	}
	return proposals
}

// Find は識別子で提案を 1 件返す。
func (s *MemoryAssistProposalStore) Find(proposalId string) (core.AssistProposal, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	proposal, found := s.byId[proposalId]
	if !found {
		return core.AssistProposal{}, false
	}
	return clonedProposal(proposal), true
}

// Create は新しい提案を 1 件記録する。
func (s *MemoryAssistProposalStore) Create(draft AssistProposalDraft) (core.AssistProposal, error) {
	if !s.available {
		return core.AssistProposal{}, ErrAssistUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ordinal == math.MaxInt64 {
		return core.AssistProposal{}, fmt.Errorf("creating the assist proposal: the ordinals are exhausted: %w",
			ErrAssertionStoreFailure)
	}
	ordinal := s.ordinal + 1
	proposal := core.AssistProposal{
		Id: assistProposalIdOf(draft, ordinal), Target: draft.Target, Note: draft.Note,
		RecordRefs: slices.Clone(draft.RecordRefs), ConversationId: draft.ConversationId, TurnId: draft.TurnId,
		Provider: draft.Provider, Model: draft.Model, AddsRelation: draft.AddsRelation,
		MatchConditions: slices.Clone(draft.MatchConditions), State: core.AssistProposalStateProposed,
		CreatedAt: s.clock.Now(),
	}
	if err := proposal.Validate(); err != nil {
		return core.AssistProposal{}, fmt.Errorf("creating the assist proposal: %w", err)
	}
	if _, taken := s.byId[proposal.Id]; taken {
		return core.AssistProposal{}, fmt.Errorf("creating the assist proposal: the identifier is already taken: %w",
			ErrAssertionStoreFailure)
	}
	if s.journal.created != nil {
		if err := s.journal.created(clonedProposal(proposal), ordinal); err != nil {
			return core.AssistProposal{}, fmt.Errorf("creating the assist proposal: %w: %w",
				ErrAssertionStoreFailure, err)
		}
	}
	s.ordinal = ordinal
	s.byId[proposal.Id] = proposal
	s.order = append(s.order, proposal.Id)
	return clonedProposal(proposal), nil
}

// Adopt は提案を採用に変え、分析者を著者とする所見を作る。
//
// **提案の状態の変更と所見の作成を、1 つの書き込みで確定する。** 書けなかったときは、提案も所見も
// 変えない。作った所見は提案の対象と根拠のレコードを引き継ぎ、元の提案の識別子を持つ。
func (s *MemoryAssistProposalStore) Adopt(
	proposalId string, adoption AssistProposalAdoption,
) (core.AssistProposal, core.Assertion, error) {
	if !s.available {
		return core.AssistProposal{}, core.Assertion{}, ErrAssistUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.proposedLocked(proposalId)
	if err != nil {
		return current, core.Assertion{}, err
	}
	var adopted core.AssistProposal
	assertion, err := s.assertions.createRecording(AssertionDraft{
		Target: current.Target, Author: adoption.Analyst,
		Basis:        core.AssertionBasis{Note: adoption.Note, RecordRefs: slices.Clone(current.RecordRefs)},
		AddsRelation: adoption.AddsRelation, ProposalId: current.Id,
	}, func(assertion core.Assertion, ordinal int64) error {
		adopted = decidedProposal(current, core.AssistProposalStateAdopted, core.AssistProposalDecision{
			Analyst: assertion.Author, DecidedAt: assertion.RecordedAt, AssertionId: assertion.Id,
		})
		if err := adopted.Validate(); err != nil {
			return fmt.Errorf("adopting the assist proposal: %w", err)
		}
		if s.journal.adopted == nil {
			return nil
		}
		return s.journal.adopted(clonedProposal(adopted), assertion, ordinal)
	})
	if err != nil {
		return core.AssistProposal{}, core.Assertion{}, fmt.Errorf("adopting the assist proposal: %w", err)
	}
	s.byId[proposalId] = adopted
	return clonedProposal(adopted), assertion, nil
}

// Reject は提案を却下に変える。
func (s *MemoryAssistProposalStore) Reject(proposalId, analyst, reason string) (core.AssistProposal, error) {
	if !s.available {
		return core.AssistProposal{}, ErrAssistUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.proposedLocked(proposalId)
	if err != nil {
		return current, err
	}
	rejected := decidedProposal(current, core.AssistProposalStateRejected, core.AssistProposalDecision{
		Analyst: analyst, DecidedAt: s.clock.Now(), Reason: reason,
	})
	if err := rejected.Validate(); err != nil {
		return core.AssistProposal{}, fmt.Errorf("rejecting the assist proposal: %w", err)
	}
	if s.journal.rejected != nil {
		if err := s.journal.rejected(clonedProposal(rejected)); err != nil {
			return core.AssistProposal{}, fmt.Errorf("rejecting the assist proposal: %w: %w",
				ErrAssertionStoreFailure, err)
		}
	}
	s.byId[proposalId] = rejected
	return clonedProposal(rejected), nil
}

// proposedLocked は提案中の提案を返す。提案が無ければ ErrAssistProposalNotFound を、提案中でなければ
// 現在の提案と ErrAssistProposalDecided を返す。呼び出し元が s.mu を持つ。
func (s *MemoryAssistProposalStore) proposedLocked(proposalId string) (core.AssistProposal, error) {
	current, found := s.byId[proposalId]
	if !found {
		return core.AssistProposal{}, ErrAssistProposalNotFound
	}
	if current.State != core.AssistProposalStateProposed {
		return clonedProposal(current), ErrAssistProposalDecided
	}
	return clonedProposal(current), nil
}

// decidedProposal は proposal を state に変え、採否の記録を付けた複製を返す。
func decidedProposal(
	proposal core.AssistProposal, state core.AssistProposalState, decision core.AssistProposalDecision,
) core.AssistProposal {
	decided := clonedProposal(proposal)
	decided.State, decided.Decision = state, &decision
	return decided
}

// assistProposalIdOf は提案の対象と会話と通番から提案の識別子を作る。
func assistProposalIdOf(draft AssistProposalDraft, ordinal int64) string {
	parts := slices.Concat(draft.Target.DigestParts(),
		[]string{draft.ConversationId, draft.TurnId, strconv.FormatInt(ordinal, 10)})
	return assistProposalIdPrefix + identityDigest(parts)
}

// clonedProposal は保存先の組と領域を共有しない複製を返す。
func clonedProposal(proposal core.AssistProposal) core.AssistProposal {
	proposal.RecordRefs = slices.Clone(proposal.RecordRefs)
	proposal.MatchConditions = slices.Clone(proposal.MatchConditions)
	if proposal.Target.Edge != nil {
		edge := *proposal.Target.Edge
		proposal.Target.Edge = &edge
	}
	if proposal.Target.Record != nil {
		record := *proposal.Target.Record
		proposal.Target.Record = &record
	}
	if proposal.Decision != nil {
		decision := *proposal.Decision
		proposal.Decision = &decision
	}
	return proposal
}
