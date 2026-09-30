package pipeline

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// assertionIdPrefix は所見の識別子を、ノードとエッジの識別子と文字列で分ける。
const assertionIdPrefix = "as:"

// ErrAssertionNotFound は、識別子に該当する所見が保存先に無いことを表す。
var ErrAssertionNotFound = errors.New("assertion store: the identifier matches no assertion")

// ErrSourceAlreadyInterpreted は、同じ収集元を指す所見が保存先に既にあることを表す。
//
// **収集元 1 件の時刻の解釈を 1 件の所見とその改訂で保つ。** 変更と取り消しは既存の所見へ改訂を足す。
var ErrSourceAlreadyInterpreted = errors.New(
	"assertion store: the source already carries a time interpretation; add a revision to it")

// ErrAssertionRevisionConflict は、改訂が元にした revision が所見の現在の revision と違うことを表す。
//
// 別の分析者が先に改訂を足した状態であり、Revise は現在の所見を一緒に返す。
var ErrAssertionRevisionConflict = errors.New(
	"assertion store: the revision is based on a superseded revision of the assertion")

// ErrAssertionStoreFailure は保存先の内部の不変条件が破れたことを表す。
//
// **要求の不備と分けるための error である。** 要求が与えた項目の不備は本 error を包まない。
// 呼び出し元は 2 つを別の応答にする。
var ErrAssertionStoreFailure = errors.New("assertion store: the store cannot hold the assertion")

// AssertionClock は所見を記録した時刻を出す port である。
//
// core に置かないのは、実装が時刻取得を必要とし、core が I/O を持てないためである。
type AssertionClock interface {
	// Now は所見を記録した時刻を返す。
	Now() core.AssertionTime
}

// SystemClock はホストの時計から記録の時刻を出す。
type SystemClock struct{}

// Now はホストの時計が指す時刻を UTC で返す。
func (SystemClock) Now() core.AssertionTime {
	return core.NewAssertionTime(time.Now())
}

// AssertionDraft は新しい所見 1 件の入力である。
// 識別子・時刻・改訂の番号・状態は保存先が決める。
type AssertionDraft struct {
	// Target は所見が指す対象である。
	Target core.AssertionTarget
	// Author は所見を記録する分析者である。
	Author string
	// Basis は所見の根拠である。
	Basis core.AssertionBasis
	// TimeOffset は収集元の時刻を読む UTC からのずれである。対象が収集元のときに持つ。
	TimeOffset *core.UtcOffset
	// AddsRelation は、記録する時点のグラフが対象の関係を持たないことである。
	AddsRelation bool
	// ProposalId は、AI 提案の採用で作る所見の元の提案の識別子である。
	ProposalId string
}

// AssertionRevisionDraft は既存の所見を置き換える新しい改訂の入力である。
// 改訂の番号と時刻は保存先が決める。
type AssertionRevisionDraft struct {
	// State は新しい改訂の時点で所見を主張するかである。
	State core.AssertionState
	// Author は新しい改訂を記録する分析者である。
	Author string
	// Basis は新しい改訂の根拠である。
	Basis core.AssertionBasis
	// TimeOffset は新しい改訂で収集元の時刻を読む UTC からのずれである。対象が収集元のときに持つ。
	TimeOffset *core.UtcOffset
	// BaseRevision は分析者が改訂の元にした所見の revision の番号である。
	BaseRevision int64
}

// AssertionStore は分析者の所見を保つ保存先の port である。
//
// **取り込み結果を書き換えない。** 所見は対象を識別子で指すだけで、原資料と自動導出は
// 本 port の外にある。
type AssertionStore interface {
	// List は記録した順に所見を返す。
	List() []core.Assertion
	// Find は識別子で所見を 1 件返す。ok が偽になるのは、該当する所見が無いときである。
	Find(assertionId string) (core.Assertion, bool)
	// Create は新しい所見を 1 件記録し、記録した所見を返す。
	Create(draft AssertionDraft) (core.Assertion, error)
	// Revise は既存の所見へ新しい改訂を足し、置き換えられた改訂を履歴へ移す。
	// 該当する所見が無いときは ErrAssertionNotFound を返す。
	Revise(assertionId string, draft AssertionRevisionDraft) (core.Assertion, error)
}

// InvestigationStore は調査 1 件の取り込み結果と、分析者が入れた値を出す保存先の port
// である。
//
// **api は本 port だけを握る。** 取り込み結果を直に握る形にすると、保存先を別の実装へ
// 差し替えたときに api を書き換える必要が出る。
type InvestigationStore interface {
	// ImportResult は取り込み結果を返す。
	ImportResult() ImportResult
	// Assertions は分析者の所見の保存先を返す。
	Assertions() AssertionStore
	// TerminalAssignments は分析者が与えた端末の割当の保存先を返す。
	TerminalAssignments() TerminalAssignmentStore
	// Assist は AI 支援の記録の保存先を返す。
	Assist() AssistStore
	// AssistProposals は AI 提案の保存先を返す。
	AssistProposals() AssistProposalStore
}

// MemoryAssertionStore は AssertionStore をメモリの上で満たす。
//
// journal を持つ store は、所見をメモリに確定する前に調査の保存先へ書く。journal を持たない
// store は、停止すると所見が消える。
type MemoryAssertionStore struct {
	clock   AssertionClock
	journal assertionJournal
	mu      sync.Mutex
	// order は記録した順の識別子である。
	order []string
	// byId は識別子から所見を探す。
	byId map[string]core.Assertion
	// ordinal は記録した所見の通番である。同じ対象の所見を別の識別子にする材料になる。
	ordinal int64
}

// NewMemoryAssertionStore はメモリ上の所見の保存先を返す。
func NewMemoryAssertionStore(clock AssertionClock) *MemoryAssertionStore {
	return &MemoryAssertionStore{clock: clock, byId: make(map[string]core.Assertion)}
}

// assertionJournal は、所見をメモリに確定する前に調査の保存先へ書く関数の組である。
//
// **保存先へ書けた所見だけを確定する。** 書けなかった所見をメモリにだけ残すと、分析者は
// 記録できたと読み、次の起動で所見が消える。
type assertionJournal struct {
	// created は新しい所見と、その識別子を作った通番を書く。
	created func(assertion core.Assertion, ordinal int64) error
	// revised は既存の所見の新しい現在の改訂を書く。
	revised func(assertion core.Assertion) error
}

// recordedAssertion は調査の保存先から読み戻した所見 1 件と、その識別子を作った通番である。
type recordedAssertion struct {
	assertion core.Assertion
	ordinal   int64
}

// newJournaledAssertionStore は、調査の保存先から読み戻した所見を記録した順に持ち、以後の
// 記録を journal へ書く store を返す。
//
// **次の通番は読み戻した通番の最大の次にする。** 同じ対象へ付けた次の所見が、読み戻した
// 所見と同じ識別子にならない。
func newJournaledAssertionStore(
	clock AssertionClock, journal assertionJournal, recorded []recordedAssertion,
) (*MemoryAssertionStore, error) {
	store := &MemoryAssertionStore{clock: clock, journal: journal, byId: make(map[string]core.Assertion)}
	for _, item := range recorded {
		if err := item.assertion.Validate(); err != nil {
			return nil, fmt.Errorf("restoring the assertion %q: %w", item.assertion.Id, err)
		}
		if _, taken := store.byId[item.assertion.Id]; taken {
			return nil, fmt.Errorf("restoring the assertion %q: the identifier is recorded twice: %w",
				item.assertion.Id, ErrAssertionStoreFailure)
		}
		store.byId[item.assertion.Id] = clonedAssertion(item.assertion)
		store.order = append(store.order, item.assertion.Id)
		store.ordinal = max(store.ordinal, item.ordinal)
	}
	return store, nil
}

// List は記録した順に所見を返す。返した組の履歴は保存先の組と別の領域にある。
func (s *MemoryAssertionStore) List() []core.Assertion {
	s.mu.Lock()
	defer s.mu.Unlock()
	assertions := make([]core.Assertion, 0, len(s.order))
	for _, id := range s.order {
		assertions = append(assertions, clonedAssertion(s.byId[id]))
	}
	return assertions
}

// Find は識別子で所見を 1 件返す。
func (s *MemoryAssertionStore) Find(assertionId string) (core.Assertion, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	assertion, found := s.byId[assertionId]
	if !found {
		return core.Assertion{}, false
	}
	return clonedAssertion(assertion), true
}

// Create は新しい所見を 1 件記録する。
//
// 同じ収集元を指す所見が既にあるときは ErrSourceAlreadyInterpreted を返す。確かめるのは
// 記録と同じ lock の中であり、同時に届いた 2 件の記録の両方を通さない。
func (s *MemoryAssertionStore) Create(draft AssertionDraft) (core.Assertion, error) {
	return s.createRecording(draft, s.journal.created)
}

// createRecording は新しい所見を 1 件作り、write が書けたときだけ確定する。write が nil のときは
// メモリにだけ確定する。
//
// **AI 提案の採用は write に、提案の状態の変更と所見の作成を 1 つの transaction で書く関数を渡す。**
// write が失敗したときは、所見も通番も確定しない。
func (s *MemoryAssertionStore) createRecording(
	draft AssertionDraft, write func(assertion core.Assertion, ordinal int64) error,
) (core.Assertion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interpretsSource(draft.Target) {
		return core.Assertion{}, ErrSourceAlreadyInterpreted
	}
	if s.ordinal == math.MaxInt64 {
		return core.Assertion{}, fmt.Errorf("creating the assertion: the ordinals are exhausted: %w",
			ErrAssertionStoreFailure)
	}
	s.ordinal++
	assertion := core.Assertion{
		Id:             assertionIdOf(draft.Target, s.ordinal),
		Target:         draft.Target,
		State:          core.AssertionStateActive,
		Author:         draft.Author,
		RecordedAt:     s.clock.Now(),
		Basis:          draft.Basis,
		TimeOffset:     clonePointer(draft.TimeOffset),
		AddsRelation:   draft.AddsRelation,
		ProposalId:     draft.ProposalId,
		RevisionNumber: core.FirstAssertionRevisionNumber,
		History:        []core.AssertionRevision{},
	}
	if err := assertion.Validate(); err != nil {
		return core.Assertion{}, fmt.Errorf("creating the assertion: %w", err)
	}
	if _, taken := s.byId[assertion.Id]; taken {
		return core.Assertion{}, fmt.Errorf(
			"creating the assertion: the identifier is already taken: %w", ErrAssertionStoreFailure)
	}
	if write != nil {
		if err := write(clonedAssertion(assertion), s.ordinal); err != nil {
			// 書けなかった所見には通番を使わない。
			s.ordinal--
			return core.Assertion{}, fmt.Errorf("creating the assertion: %w: %w", ErrAssertionStoreFailure, err)
		}
	}
	s.byId[assertion.Id] = assertion
	s.order = append(s.order, assertion.Id)
	return clonedAssertion(assertion), nil
}

// interpretsSource は、target が収集元であり、同じ収集元を指す所見を保存先が持つかを返す。
// 呼び出し元が s.mu を持つ。
func (s *MemoryAssertionStore) interpretsSource(target core.AssertionTarget) bool {
	if target.Kind != core.AssertionTargetKindSource {
		return false
	}
	for _, id := range s.order {
		existing := s.byId[id].Target
		if existing.Kind == core.AssertionTargetKindSource &&
			existing.SourceContentSha256 == target.SourceContentSha256 {
			return true
		}
	}
	return false
}

// Revise は既存の所見へ新しい改訂を足す。
//
// **古い改訂を消さない。** 置き換えられた改訂は履歴の末尾に入り、著者と時刻と根拠を保つ。
// draft.BaseRevision が現在の revision と違うときは ErrAssertionRevisionConflict と現在の所見を返す。
func (s *MemoryAssertionStore) Revise(
	assertionId string, draft AssertionRevisionDraft,
) (core.Assertion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, found := s.byId[assertionId]
	if !found {
		return core.Assertion{}, ErrAssertionNotFound
	}
	// **先の改訂を通知せずに置き換えない。** 元にした revision が古い改訂は、現在の所見と並べて
	// 分析者に採る値を選ばせる。
	if current.RevisionNumber != draft.BaseRevision {
		return clonedAssertion(current), ErrAssertionRevisionConflict
	}
	if current.RevisionNumber == math.MaxInt64 {
		return core.Assertion{}, fmt.Errorf(
			"revising the assertion: the revisions are exhausted: %w", ErrAssertionStoreFailure)
	}
	superseded := core.AssertionRevision{
		RevisionNumber: current.RevisionNumber,
		State:          current.State,
		Author:         current.Author,
		RecordedAt:     current.RecordedAt,
		Basis:          current.Basis,
		TimeOffset:     current.TimeOffset,
	}
	revised := current
	revised.History = append(clonedRevisions(current.History), superseded)
	revised.RevisionNumber = current.RevisionNumber + 1
	revised.State = draft.State
	revised.Author = draft.Author
	revised.RecordedAt = s.clock.Now()
	revised.Basis = draft.Basis
	revised.TimeOffset = clonePointer(draft.TimeOffset)
	if err := revised.Validate(); err != nil {
		return core.Assertion{}, fmt.Errorf("revising the assertion: %w", err)
	}
	if s.journal.revised != nil {
		if err := s.journal.revised(clonedAssertion(revised)); err != nil {
			return core.Assertion{}, fmt.Errorf("revising the assertion: %w: %w", ErrAssertionStoreFailure, err)
		}
	}
	s.byId[assertionId] = revised
	return clonedAssertion(revised), nil
}

// MemoryStore は InvestigationStore をメモリの上で満たす。
type MemoryStore struct {
	result      ImportResult
	assertions  *MemoryAssertionStore
	assignments *MemoryTerminalAssignmentStore
	assist      AssistStore
	proposals   *MemoryAssistProposalStore
}

// NewMemoryStore は取り込み結果と、メモリ上の所見と割当の保存先を組にして返す。AI 支援と AI 提案は受け付けない。
//
// **停止で消える記録を監査の記録として扱わない。**
func NewMemoryStore(result ImportResult, clock AssertionClock) *MemoryStore {
	return &MemoryStore{
		result:      result,
		assertions:  NewMemoryAssertionStore(clock),
		assignments: NewMemoryTerminalAssignmentStore(),
		assist:      UnavailableAssistStore(),
		proposals:   NewUnavailableAssistProposalStore(),
	}
}

// NewMemoryStoreWithAssistProposals は、NewMemoryStore に AI 提案をメモリの上で保つ保存先を加えて返す。
//
// **停止で提案が消えるため、server の起動の組み立ては使わない。** api の test が、調査の file を
// 作らずに AI 提案の操作を確かめるために使う。
func NewMemoryStoreWithAssistProposals(result ImportResult, clock AssertionClock) (*MemoryStore, error) {
	store := NewMemoryStore(result, clock)
	proposals, err := newJournaledAssistProposalStore(clock, store.assertions, assistProposalJournal{}, nil)
	if err != nil {
		return nil, err
	}
	store.proposals = proposals
	return store, nil
}

// ImportResult は取り込み結果を返す。
func (s *MemoryStore) ImportResult() ImportResult { return s.result }

// newStoreOf は取り込み結果と、組み立て済みの所見と割当と AI 支援の記録と AI 提案の保存先を組にして返す。
func newStoreOf(
	result ImportResult, assertions *MemoryAssertionStore, assignments *MemoryTerminalAssignmentStore,
	assist AssistStore, proposals *MemoryAssistProposalStore,
) *MemoryStore {
	return &MemoryStore{result: result, assertions: assertions, assignments: assignments, assist: assist, proposals: proposals}
}

// Assist は AI 支援の記録の保存先を返す。
func (s *MemoryStore) Assist() AssistStore { return s.assist }

// Assertions は分析者の所見の保存先を返す。
func (s *MemoryStore) Assertions() AssertionStore { return s.assertions }

// TerminalAssignments は分析者が与えた端末の割当の保存先を返す。
func (s *MemoryStore) TerminalAssignments() TerminalAssignmentStore { return s.assignments }

// AssistProposals は AI 提案の保存先を返す。
func (s *MemoryStore) AssistProposals() AssistProposalStore { return s.proposals }

// assertionIdOf は対象と通番から所見の識別子を作る。
//
// 通番を材料に入れるのは、同じ対象へ所見を 2 件付けられるようにするためである。
func assertionIdOf(target core.AssertionTarget, ordinal int64) string {
	parts := make([]string, 0, len(target.DigestParts())+1)
	parts = append(parts, target.DigestParts()...)
	parts = append(parts, strconv.FormatInt(ordinal, 10))
	return assertionIdPrefix + identityDigest(parts)
}

// clonedAssertion は保存先の組と領域を共有しない複製を返す。
// 呼び出し元が履歴と根拠の要素を書き換えても保存先の組は変わらない。
func clonedAssertion(assertion core.Assertion) core.Assertion {
	assertion.History = clonedRevisions(assertion.History)
	assertion.Basis.RecordRefs = slices.Clone(assertion.Basis.RecordRefs)
	assertion.TimeOffset = clonePointer(assertion.TimeOffset)
	if assertion.Target.Edge != nil {
		edge := *assertion.Target.Edge
		assertion.Target.Edge = &edge
	}
	if assertion.Target.Record != nil {
		record := *assertion.Target.Record
		assertion.Target.Record = &record
	}
	return assertion
}

// clonedRevisions は履歴の要素ごとに根拠の集合も複製する。
func clonedRevisions(revisions []core.AssertionRevision) []core.AssertionRevision {
	cloned := slices.Clone(revisions)
	for index := range cloned {
		cloned[index].Basis.RecordRefs = slices.Clone(cloned[index].Basis.RecordRefs)
		cloned[index].TimeOffset = clonePointer(cloned[index].TimeOffset)
	}
	return cloned
}
