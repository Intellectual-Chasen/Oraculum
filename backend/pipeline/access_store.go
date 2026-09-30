package pipeline

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// Role は調査に参加する利用者の役割である。
type Role string

// Role の値。後の値ほど多くの操作を許す。
const (
	// RoleViewer は証拠と原文を読み、自分のワークスペースを保存・共有する。
	RoleViewer Role = "viewer"
	// RoleEditor は閲覧者の操作に加えて、分析者の判断と調査条件を記録し、調査の段階を始める。
	RoleEditor Role = "editor"
	// RoleAdmin は編集者の操作に加えて、利用者の役割を管理する。
	RoleAdmin Role = "admin"
)

// Roles は Role の値を、許す操作の少ない順に返す。
func Roles() []Role { return []Role{RoleViewer, RoleEditor, RoleAdmin} }

// IsKnown は Role が定義の中の値であるかを返す。
func (r Role) IsKnown() bool { return slices.Contains(Roles(), r) }

// Includes は、r の利用者が required の役割の操作を行えるかを返す。
func (r Role) Includes(required Role) bool {
	return r.IsKnown() && slices.Index(Roles(), r) >= slices.Index(Roles(), required)
}

// OperatorActor は、運用者の起動引数が与えた役割の記録に書く、与えた利用者の名前である。
const OperatorActor = "--admin"

var (
	// ErrMemberNotFound は、役割を持たない利用者を指したことを表す。
	ErrMemberNotFound = errors.New("access store: the login has no role")
	// ErrLastAdmin は、最後の管理者の役割を外そうとしたことを表す。
	ErrLastAdmin = errors.New("access store: the investigation keeps at least one admin")
	// ErrUnknownRole は、定義の外の役割を指したことを表す。
	ErrUnknownRole = errors.New("access store: the role is not known")
)

// Member は調査に参加する利用者 1 人の役割である。
type Member struct {
	Login     string
	Role      Role
	GrantedBy string
	GrantedAt time.Time
}

// AuditEvent は、権限と共有の変更 1 件の記録である。
type AuditEvent struct {
	RecordedAt time.Time
	Actor      string
	Action     string
	Target     string
	Detail     string
}

// 監査の記録の操作の種類。
const (
	AuditMemberGranted = "member_granted"
	AuditMemberRevoked = "member_revoked"
)

// accessJournal は、役割の変更を調査の保存先へ書く。書けなければ変更をメモリに確定しない。
type accessJournal func(members []Member, removed []string, events []AuditEvent) error

// AccessStore は調査に参加する利用者の役割を保つ。
//
// journal を持たない store は、変更と監査の記録をメモリに溜める。調査の file を作る時点で
// Investigation.Settle が溜めた値を書き、以後は変更ごとに書く。**調査の file を作る前に停止すると、
// 溜めた変更は消える。**
type AccessStore struct {
	mu      sync.Mutex
	now     func() time.Time
	members map[string]Member
	journal accessJournal
	// pending は journal を持つ前に記録した監査の記録である。
	pending []AuditEvent
}

// NewAccessStore は members の役割を持つ store を返す。
func NewAccessStore(members []Member) *AccessStore {
	store := &AccessStore{now: time.Now, members: map[string]Member{}}
	for _, member := range members {
		store.members[member.Login] = member
	}
	return store
}

// Role は login の役割を返す。役割を持たない利用者では ok が偽である。
func (s *AccessStore) Role(login string) (Role, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	member, ok := s.members[login]
	return member.Role, ok
}

// Member は login の役割と、与えた利用者と時刻を返す。役割を持たない利用者では ok が偽である。
func (s *AccessStore) Member(login string) (Member, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	member, ok := s.members[login]
	return member, ok
}

// List は役割を持つ利用者を、ログイン名の順に返す。
func (s *AccessStore) List() []Member {
	s.mu.Lock()
	defer s.mu.Unlock()
	members := make([]Member, 0, len(s.members))
	for _, member := range s.members {
		members = append(members, member)
	}
	slices.SortFunc(members, func(a, b Member) int { return strings.Compare(a.Login, b.Login) })
	return members
}

// Grant は actor が login に role を与える。既に役割を持つ利用者は置き換える。
func (s *AccessStore) Grant(actor, login string, role Role) (Member, error) {
	if !role.IsKnown() {
		return Member{}, ErrUnknownRole
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous, ok := s.members[login]; ok && previous.Role == RoleAdmin && role != RoleAdmin && s.adminCount() == 1 {
		return Member{}, ErrLastAdmin
	}
	now := s.now().UTC()
	member := Member{Login: login, Role: role, GrantedBy: actor, GrantedAt: now}
	event := AuditEvent{RecordedAt: now, Actor: actor, Action: AuditMemberGranted, Target: login, Detail: string(role)}
	if err := s.write([]Member{member}, nil, event); err != nil {
		return Member{}, err
	}
	s.members[login] = member
	return member, nil
}

// Revoke は actor が login の役割を外す。
func (s *AccessStore) Revoke(actor, login string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, ok := s.members[login]
	if !ok {
		return ErrMemberNotFound
	}
	if previous.Role == RoleAdmin && s.adminCount() == 1 {
		return ErrLastAdmin
	}
	event := AuditEvent{RecordedAt: s.now().UTC(), Actor: actor, Action: AuditMemberRevoked, Target: login}
	if err := s.write(nil, []string{login}, event); err != nil {
		return err
	}
	delete(s.members, login)
	return nil
}

// write は変更を保存先へ書く。journal を持たない store は監査の記録を溜める。呼び出し側が mu を持つ。
func (s *AccessStore) write(members []Member, removed []string, event AuditEvent) error {
	if s.journal == nil {
		s.pending = append(s.pending, event)
		return nil
	}
	if err := s.journal(members, removed, []AuditEvent{event}); err != nil {
		return fmt.Errorf("access store: recording the change: %w", err)
	}
	return nil
}

// attach は、溜めた役割と監査の記録を journal で書き、以後の変更を journal で書く。
//
// 既に journal を持つ store では何もしない。書けなかった attach は、次の Settle でやり直す。
func (s *AccessStore) attach(journal accessJournal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.journal != nil {
		return nil
	}
	members := make([]Member, 0, len(s.members))
	for _, member := range s.members {
		members = append(members, member)
	}
	if len(members) > 0 || len(s.pending) > 0 {
		if err := journal(members, nil, s.pending); err != nil {
			return fmt.Errorf("access store: recording the roles: %w", err)
		}
	}
	s.journal, s.pending = journal, nil
	return nil
}

func (s *AccessStore) adminCount() int {
	count := 0
	for _, member := range s.members {
		if member.Role == RoleAdmin {
			count++
		}
	}
	return count
}
