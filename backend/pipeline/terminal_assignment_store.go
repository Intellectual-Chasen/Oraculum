package pipeline

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ErrTerminalAssignmentNotStorable は割当を保存先が受け付けないことを表す。
var ErrTerminalAssignmentNotStorable = errors.New(
	"terminal assignment store: the store cannot hold the assignment")

// ErrTerminalAssignmentAlreadyRecorded は、同じ内容の割当を既に記録していることを表す。
var ErrTerminalAssignmentAlreadyRecorded = errors.New(
	"terminal assignment store: the store already holds the same assignment")

// TerminalAssignmentDraft は分析者が与える割当 1 件の入力である。
//
// 由来は保存先が analyst_supplied に決める。収集元が記録した割当は取り込み結果から出る
// ものであり、本 port を通らない。
type TerminalAssignmentDraft struct {
	// ClientIp は端末に割り当てる接続元 IP である。
	ClientIp string
	// TerminalId は端末の外部識別子である。
	TerminalId string
	// TerminalHostname は分析者が読む端末の表示名である。
	TerminalHostname string
	// TerminalHostnames は端末が名乗るホスト名 (短い名前と FQDN) の並びである。
	TerminalHostnames []string
	// SourceId は適用期間を読み取った収集元の取り込み 1 件である。
	SourceId string
	// SourceContentSha256 は同じ収集元の内容の識別である。
	SourceContentSha256 string
	// AssignmentValidRange は割当を適用してよい期間である。
	AssignmentValidRange core.TimeRange
	// Derivation は分析者が割当を導いた筋道である。
	Derivation string
	// BasisRecordRefs は分析者が根拠に挙げたレコードである。
	BasisRecordRefs []core.AssertionRecordRef
	// Author は割当を与える分析者である。
	Author string
	// AppliesToSourceId は、レコードの全体がこの端末のものである収集元である。
	AppliesToSourceId string
}

// assignment は入力を、由来を analyst_supplied にした割当へ直す。
func (d TerminalAssignmentDraft) assignment() core.TerminalAssignment {
	return core.TerminalAssignment{
		ClientIp:             d.ClientIp,
		TerminalId:           d.TerminalId,
		TerminalHostname:     d.TerminalHostname,
		TerminalHostnames:    slices.Clone(d.TerminalHostnames),
		SourceId:             d.SourceId,
		SourceContentSha256:  d.SourceContentSha256,
		AssignmentValidRange: d.AssignmentValidRange,
		Origin:               core.TerminalAssignmentOriginAnalystSupplied,
		Derivation:           d.Derivation,
		BasisRecordRefs:      slices.Clone(d.BasisRecordRefs),
		Author:               d.Author,
		AppliesToSourceId:    d.AppliesToSourceId,
	}
}

// TerminalAssignmentStore は分析者が与えた端末の割当を保つ保存先の port である。
//
// **取り込み結果を書き換えない。** 割当は接続元 IP と端末の組を足すだけで、原資料と
// 自動導出は本 port の外にある。
type TerminalAssignmentStore interface {
	// List は記録した順の割当と、その組に対応する番号を返す。
	//
	// **組と番号を 1 回の呼び出しで返す。** 2 回に分けると、その間に記録された割当を
	// 含む組へ古い番号が付き、次の要求が同じ組を組み直す。
	List() ([]core.TerminalAssignment, int64)
	// Create は新しい割当を 1 件記録し、記録した割当を返す。
	Create(draft TerminalAssignmentDraft) (core.TerminalAssignment, error)
	// Revision は記録した割当が変わるたびに増える番号を返す。
	//
	// 割当は端末の判定の材料であり、索引とグラフを組み直す必要がある。読む側は本値の
	// 変化で組み直す時点を決める。
	Revision() int64
}

// MemoryTerminalAssignmentStore は TerminalAssignmentStore をメモリの上で満たす。
//
// journal を持つ store は、割当をメモリに確定する前に調査の保存先へ書く。journal を持たない
// store は、停止すると割当が消える。
type MemoryTerminalAssignmentStore struct {
	// journal は割当を確定する前に調査の保存先へ書く。**書けた割当だけを確定する。**
	journal  func(assignment core.TerminalAssignment) error
	mu       sync.Mutex
	entries  []core.TerminalAssignment
	revision int64
}

// NewMemoryTerminalAssignmentStore はメモリ上の割当の保存先を返す。
func NewMemoryTerminalAssignmentStore() *MemoryTerminalAssignmentStore {
	return &MemoryTerminalAssignmentStore{}
}

// newJournaledTerminalAssignmentStore は、調査の保存先から読み戻した割当を記録した順に持ち、
// 以後の記録を journal へ書く store を返す。
func newJournaledTerminalAssignmentStore(
	journal func(core.TerminalAssignment) error, recorded []core.TerminalAssignment,
) (*MemoryTerminalAssignmentStore, error) {
	for index, assignment := range recorded {
		if assignment.Origin != core.TerminalAssignmentOriginAnalystSupplied {
			return nil, fmt.Errorf("restoring the terminal assignment %d: the origin is %q: %w",
				index, assignment.Origin, ErrTerminalAssignmentNotStorable)
		}
		if err := assignment.Validate(); err != nil {
			return nil, fmt.Errorf("restoring the terminal assignment %d: %w", index, err)
		}
	}
	return &MemoryTerminalAssignmentStore{
		journal: journal, entries: cloneAssignments(recorded), revision: int64(len(recorded)),
	}, nil
}

// List は記録した順の割当と、その組に対応する番号を返す。
// 返した組は保存先の組と別の領域にある。
func (s *MemoryTerminalAssignmentStore) List() ([]core.TerminalAssignment, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneAssignments(s.entries), s.revision
}

// Create は新しい割当を 1 件記録する。
//
// **検査を通らない割当を保存しない。** 導いた筋道と根拠のレコードを欠く推定を、
// 収集元の観測と同じ形で並べない。
func (s *MemoryTerminalAssignmentStore) Create(
	draft TerminalAssignmentDraft,
) (core.TerminalAssignment, error) {
	assignment := draft.assignment()
	if err := assignment.Validate(); err != nil {
		return core.TerminalAssignment{}, fmt.Errorf("%w: %w",
			ErrTerminalAssignmentNotStorable, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if slices.ContainsFunc(s.entries, func(held core.TerminalAssignment) bool {
		return sameAssignment(held, assignment)
	}) {
		return core.TerminalAssignment{}, ErrTerminalAssignmentAlreadyRecorded
	}
	if s.journal != nil {
		if err := s.journal(cloneAssignments([]core.TerminalAssignment{assignment})[0]); err != nil {
			return core.TerminalAssignment{}, fmt.Errorf("recording the terminal assignment: %w", err)
		}
	}
	s.entries = append(s.entries, assignment)
	s.revision++
	return cloneAssignments([]core.TerminalAssignment{assignment})[0], nil
}

// sameAssignment は、2 件の割当が記録した分析者を除いて同じ内容であるかを返す。
// 期間は両端の時刻の文字列と時計まで比べる。
func sameAssignment(a, b core.TerminalAssignment) bool {
	a.Author, b.Author = "", ""
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
}

// Revision は記録した割当が変わるたびに増える番号を返す。
func (s *MemoryTerminalAssignmentStore) Revision() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revision
}

// cloneAssignments は割当の集合を、要素の中の集合ごと複製する。
func cloneAssignments(assignments []core.TerminalAssignment) []core.TerminalAssignment {
	copied := slices.Clone(assignments)
	for index := range copied {
		copied[index].BasisRecordRefs = slices.Clone(copied[index].BasisRecordRefs)
		copied[index].TerminalHostnames = slices.Clone(copied[index].TerminalHostnames)
		copied[index].AssignmentValidRange = core.TimeRange{
			From: cloneTimestamp(copied[index].AssignmentValidRange.From),
			To:   cloneTimestamp(copied[index].AssignmentValidRange.To),
		}
	}
	return copied
}
