package pipeline

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// numberedWorkspacePrefix は、名前を持たずに作ったワークスペースへ付ける名前の、番号の前の文字列である。
const numberedWorkspacePrefix = "ワークスペース "

var (
	// ErrWorkspaceNotFound は、識別子のワークスペースが無いことを表す。
	ErrWorkspaceNotFound = errors.New("workspace store: no workspace has the identifier")
	// ErrWorkspaceRevisionConflict は、更新の元にした revision より後に、別の更新を記録したことを表す。
	ErrWorkspaceRevisionConflict = errors.New("workspace store: the workspace changed after the base revision")
	// ErrWorkspaceInvalid は、名前が値の条件に合わないことを表す。
	ErrWorkspaceInvalid = errors.New("workspace store: the workspace name is invalid")
)

// Workspace は、分析者が保存した画面の状態 1 件である。
//
// **調査の原資料、導出の結果、分析者の判断を持たない。** State は画面の状態の JSON であり、
// 調査の値は識別子で指す。本 package は State の中身を解釈しない。
type Workspace struct {
	Id        string
	Owner     string
	Name      string
	State     []byte
	Revision  int64
	UpdatedAt time.Time
	UpdatedBy string
	// Shares は共有先の利用者を、ログイン名の順に並べる。
	Shares []WorkspaceShare
}

// WorkspaceAccess は、共有先の利用者がワークスペースに行える操作である。
type WorkspaceAccess string

// WorkspaceAccess の値。
const (
	// WorkspaceView は読み出しだけを許す。
	WorkspaceView WorkspaceAccess = "view"
	// WorkspaceEdit は読み出しと置き換えを許す。
	WorkspaceEdit WorkspaceAccess = "edit"
)

// WorkspaceShare は、ワークスペースを共有した利用者 1 人である。
type WorkspaceShare struct {
	Login     string
	Access    WorkspaceAccess
	GrantedBy string
	GrantedAt time.Time
}

// ワークスペースの共有の監査の記録の操作の種類。
const (
	AuditWorkspaceShared   = "workspace_shared"
	AuditWorkspaceUnshared = "workspace_unshared"
)

// ErrWorkspaceShareNotFound は、利用者への共有が無いことを表す。
var ErrWorkspaceShareNotFound = errors.New("workspace store: the workspace is not shared with the login")

// ShareOf は login への共有を返す。共有が無ければ ok が偽である。
func (w Workspace) ShareOf(login string) (WorkspaceShare, bool) {
	index := slices.IndexFunc(w.Shares, func(share WorkspaceShare) bool { return share.Login == login })
	if index < 0 {
		return WorkspaceShare{}, false
	}
	return w.Shares[index], true
}

// ワークスペースの変更の記録の欄名と、clientChangeId の長さ。
const (
	// WorkspaceAllFields は、state 全体を置き換えた変更、または変更した欄が分からないことを表す欄名である。
	WorkspaceAllFields     = "*"
	maxClientChangeIdBytes = 64
)

// WorkspaceChange は、ワークスペースの revision を 1 つ進めた変更 1 件の記録である。
type WorkspaceChange struct {
	WorkspaceId string
	Revision    int64
	// ClientChangeId は client が変更に付けた識別子である。同じワークスペースの中で一意である。
	ClientChangeId string
	Actor          string
	// Fields は変更した state の最上位の欄名を、名前の順に並べる。全体の置き換えは WorkspaceAllFields だけを持つ。
	Fields     []string
	RecordedAt time.Time
}

// WorkspaceChangeResult は Change の結果である。
type WorkspaceChangeResult struct {
	Workspace Workspace
	Change    WorkspaceChange
	// Replayed は、同じ ClientChangeId の変更を前に適用しており、今回は何も変えなかったことを表す。
	Replayed bool
}

// WorkspaceConflictError は、base の revision より後の変更が、同じ欄を変えたことを表す。
// errors.Is で ErrWorkspaceRevisionConflict に該当する。
type WorkspaceConflictError struct {
	// Fields は重なった欄名である。変更した欄が分からないときは WorkspaceAllFields だけを持つ。
	Fields  []string
	Current Workspace
}

func (e *WorkspaceConflictError) Error() string {
	return ErrWorkspaceRevisionConflict.Error() + ": " + strings.Join(e.Fields, ",")
}

func (e *WorkspaceConflictError) Unwrap() error { return ErrWorkspaceRevisionConflict }

// workspaceJournal は、ワークスペースと共有の変更、変更の記録、監査の記録を調査の保存先へ書く。
// workspaces の共有は保存先の共有を置き換える。書けなければ変更をメモリに確定しない。
type workspaceJournal func(workspaces []Workspace, removed []string, changes []WorkspaceChange, events []AuditEvent) error

// WorkspaceStore は保存したワークスペースを保つ。
//
// journal を持たない store はメモリだけに置き、共有の監査の記録を溜める。調査の file を作る時点で
// Investigation.Settle が溜めた値を書き、以後は変更ごとに書く。
type WorkspaceStore struct {
	mu         sync.Mutex
	now        func() time.Time
	workspaces map[string]Workspace
	// changes はワークスペースごとの変更の記録を、revision の順にすべて持つ。
	changes map[string][]WorkspaceChange
	journal workspaceJournal
	// pending は journal を持つ前に記録した監査の記録である。
	pending []AuditEvent
}

// NewWorkspaceStore は workspaces を持つ store を返す。
func NewWorkspaceStore(workspaces []Workspace) *WorkspaceStore {
	store := &WorkspaceStore{now: time.Now, workspaces: map[string]Workspace{}, changes: map[string][]WorkspaceChange{}}
	for _, workspace := range workspaces {
		store.workspaces[workspace.Id] = workspace
	}
	return store
}

// Get は識別子のワークスペースを返す。
func (s *WorkspaceStore) Get(id string) (Workspace, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, ok := s.workspaces[id]
	return cloneWorkspace(workspace), ok
}

// List は、include が真を返すワークスペースを、更新の新しい順に返す。
func (s *WorkspaceStore) List(include func(Workspace) bool) []Workspace {
	s.mu.Lock()
	defer s.mu.Unlock()
	var listed []Workspace
	for _, workspace := range s.workspaces {
		if include(workspace) {
			listed = append(listed, cloneWorkspace(workspace))
		}
	}
	slices.SortFunc(listed, func(a, b Workspace) int {
		if order := b.UpdatedAt.Compare(a.UpdatedAt); order != 0 {
			return order
		}
		return strings.Compare(a.Id, b.Id)
	})
	return listed
}

// Create は owner のワークスペースを作る。name が空の文字列なら、owner の「ワークスペース N」の
// 名前のうち最大の N に 1 を足した番号で名前を付ける。
func (s *WorkspaceStore) Create(owner, name string, state []byte) (Workspace, error) {
	if name != "" {
		if err := checkWorkspaceName(name); err != nil {
			return Workspace{}, err
		}
	}
	id, err := newWorkspaceId()
	if err != nil {
		return Workspace{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "" {
		name = s.nextNumberedName(owner)
	}
	workspace := Workspace{
		Id: id, Owner: owner, Name: name, State: slices.Clone(state), Revision: 1, UpdatedAt: s.now().UTC(),
		UpdatedBy: owner,
	}
	if err := s.write([]Workspace{workspace}, nil, nil); err != nil {
		return Workspace{}, err
	}
	s.workspaces[id] = workspace
	return cloneWorkspace(workspace), nil
}

// Update は、baseRevision のワークスペースを name と state に置き換え、revision を 1 つ進める。
// 置き換えは欄 WorkspaceAllFields の変更として記録する。baseRevision より後に別の更新を記録していれば、
// ErrWorkspaceRevisionConflict と現在の値を返す。
func (s *WorkspaceStore) Update(
	id, actor string, baseRevision int64, name string, state []byte,
) (Workspace, WorkspaceChange, error) {
	if err := checkWorkspaceName(name); err != nil {
		return Workspace{}, WorkspaceChange{}, err
	}
	clientChangeId, err := newWorkspaceId()
	if err != nil {
		return Workspace{}, WorkspaceChange{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.workspaces[id]
	if !ok {
		return Workspace{}, WorkspaceChange{}, ErrWorkspaceNotFound
	}
	if current.Revision != baseRevision {
		return cloneWorkspace(current), WorkspaceChange{}, ErrWorkspaceRevisionConflict
	}
	updated, change := s.advance(current, actor, "replace-"+clientChangeId, []string{WorkspaceAllFields})
	updated.Name, updated.State = name, slices.Clone(state)
	if err := s.commitChange(updated, change); err != nil {
		return Workspace{}, WorkspaceChange{}, err
	}
	return cloneWorkspace(updated), change, nil
}

// Change は、baseRevision を元にした変更として、state の最上位の欄を patch の値に置き換え、revision を
// 1 つ進める。値が null の欄は state から消す。欄の中身は解釈しない。
//
// baseRevision より後の変更が patch と同じ欄を変えていれば *WorkspaceConflictError を返す。別の欄だけが
// 変わっていれば適用する。baseRevision より後の変更を記録から辿れなければ、欄 WorkspaceAllFields の
// 競合とする。同じ clientChangeId の変更を記録していれば、何も変えずに前の結果を返す。その変更の actor が
// 別の利用者なら ErrWorkspaceInvalid を返す。
func (s *WorkspaceStore) Change(
	id, actor, clientChangeId string, baseRevision int64, patch map[string]json.RawMessage,
) (WorkspaceChangeResult, error) {
	if clientChangeId == "" || len(clientChangeId) > maxClientChangeIdBytes {
		return WorkspaceChangeResult{}, fmt.Errorf("the clientChangeId must have 1 to 64 bytes: %w", ErrWorkspaceInvalid)
	}
	if len(patch) == 0 {
		return WorkspaceChangeResult{}, fmt.Errorf("the patch must change at least one field: %w", ErrWorkspaceInvalid)
	}
	if _, all := patch[WorkspaceAllFields]; all {
		return WorkspaceChangeResult{}, fmt.Errorf("the patch cannot name the field %q: %w", WorkspaceAllFields,
			ErrWorkspaceInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.workspaces[id]
	if !ok {
		return WorkspaceChangeResult{}, ErrWorkspaceNotFound
	}
	for _, recorded := range s.changes[id] {
		if recorded.ClientChangeId == clientChangeId {
			if recorded.Actor != actor {
				return WorkspaceChangeResult{}, fmt.Errorf("another login recorded the clientChangeId: %w",
					ErrWorkspaceInvalid)
			}
			return WorkspaceChangeResult{Workspace: cloneWorkspace(current), Change: cloneChange(recorded), Replayed: true}, nil
		}
	}
	fields := slices.Sorted(maps.Keys(patch))
	if overlapping := s.overlapping(current, baseRevision, fields); len(overlapping) > 0 {
		return WorkspaceChangeResult{}, &WorkspaceConflictError{Fields: overlapping, Current: cloneWorkspace(current)}
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(current.State, &state); err != nil || state == nil {
		return WorkspaceChangeResult{}, fmt.Errorf("the stored state is not a JSON object: %w", ErrWorkspaceInvalid)
	}
	for field, value := range patch {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			delete(state, field)
			continue
		}
		state[field] = value
	}
	merged, err := json.Marshal(state)
	if err != nil {
		return WorkspaceChangeResult{}, fmt.Errorf("the patch carries a value that is not JSON: %w", ErrWorkspaceInvalid)
	}
	updated, change := s.advance(current, actor, clientChangeId, fields)
	updated.State = merged
	if err := s.commitChange(updated, change); err != nil {
		return WorkspaceChangeResult{}, err
	}
	return WorkspaceChangeResult{Workspace: cloneWorkspace(updated), Change: cloneChange(change)}, nil
}

// overlapping は、baseRevision より後の変更が変えた欄のうち fields と重なる欄を返す。重ならなければ nil
// である。呼び出し側が mu を持つ。
func (s *WorkspaceStore) overlapping(current Workspace, baseRevision int64, fields []string) []string {
	if baseRevision == current.Revision {
		return nil
	}
	all := []string{WorkspaceAllFields}
	recorded := s.changes[current.Id]
	start := slices.IndexFunc(recorded, func(change WorkspaceChange) bool { return change.Revision == baseRevision+1 })
	if baseRevision < 1 || baseRevision > current.Revision || start < 0 {
		return all
	}
	var overlapping []string
	for _, change := range recorded[start:] {
		if slices.Contains(change.Fields, WorkspaceAllFields) {
			return all
		}
		for _, field := range change.Fields {
			if slices.Contains(fields, field) && !slices.Contains(overlapping, field) {
				overlapping = append(overlapping, field)
			}
		}
	}
	slices.Sort(overlapping)
	return overlapping
}

// advance は current の revision を 1 つ進めた値と、その変更の記録を返す。呼び出し側が mu を持つ。
func (s *WorkspaceStore) advance(
	current Workspace, actor, clientChangeId string, fields []string,
) (Workspace, WorkspaceChange) {
	now := s.now().UTC()
	updated := cloneWorkspace(current)
	updated.Revision, updated.UpdatedAt, updated.UpdatedBy = current.Revision+1, now, actor
	return updated, WorkspaceChange{WorkspaceId: current.Id, Revision: updated.Revision, ClientChangeId: clientChangeId,
		Actor: actor, Fields: fields, RecordedAt: now}
}

// commitChange は updated と change を書き、メモリに確定する。呼び出し側が mu を持つ。
func (s *WorkspaceStore) commitChange(updated Workspace, change WorkspaceChange) error {
	if err := s.write([]Workspace{updated}, nil, []WorkspaceChange{change}); err != nil {
		return err
	}
	s.workspaces[updated.Id] = updated
	s.changes[updated.Id] = append(s.changes[updated.Id], change)
	return nil
}

// nextNumberedName は、owner の「ワークスペース N」の名前のうち最大の N に 1 を足した名前を返す。
// N は数字だけからなる文字列である。int の最大値の N は、1 を足すと桁があふれるので数えない。
// 呼び出し側が mu を持つ。
func (s *WorkspaceStore) nextNumberedName(owner string) string {
	largest := 0
	for _, workspace := range s.workspaces {
		digits, ok := strings.CutPrefix(workspace.Name, numberedWorkspacePrefix)
		if workspace.Owner != owner || !ok || digits == "" ||
			strings.ContainsFunc(digits, func(r rune) bool { return r < '0' || r > '9' }) {
			continue
		}
		if number, err := strconv.Atoi(digits); err == nil && number > largest && number < math.MaxInt {
			largest = number
		}
	}
	return numberedWorkspacePrefix + strconv.Itoa(largest+1)
}

// Delete は actor が識別子のワークスペースを消す。消える共有ごとに、取り消しを監査に記録する。
func (s *WorkspaceStore) Delete(id, actor string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.workspaces[id]
	if !ok {
		return ErrWorkspaceNotFound
	}
	events := make([]AuditEvent, 0, len(current.Shares))
	for _, share := range current.Shares {
		events = append(events, AuditEvent{
			RecordedAt: s.now().UTC(), Actor: actor, Action: AuditWorkspaceUnshared, Target: id + ":" + share.Login,
			Detail: "workspace_deleted",
		})
	}
	if err := s.write(nil, []string{id}, nil, events...); err != nil {
		return err
	}
	delete(s.workspaces, id)
	delete(s.changes, id)
	return nil
}

// Share は、actor が id のワークスペースを login へ access で共有する。既にある共有は置き換える。
func (s *WorkspaceStore) Share(id, actor, login string, access WorkspaceAccess) (WorkspaceShare, error) {
	if access != WorkspaceView && access != WorkspaceEdit {
		return WorkspaceShare{}, fmt.Errorf("the access must be view or edit: %w", ErrWorkspaceInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.workspaces[id]
	if !ok {
		return WorkspaceShare{}, ErrWorkspaceNotFound
	}
	now := s.now().UTC()
	share := WorkspaceShare{Login: login, Access: access, GrantedBy: actor, GrantedAt: now}
	updated := cloneWorkspace(current)
	updated.Shares = slices.DeleteFunc(updated.Shares, func(other WorkspaceShare) bool { return other.Login == login })
	updated.Shares = append(updated.Shares, share)
	slices.SortFunc(updated.Shares, func(a, b WorkspaceShare) int { return strings.Compare(a.Login, b.Login) })
	event := AuditEvent{RecordedAt: now, Actor: actor, Action: AuditWorkspaceShared, Target: id + ":" + login,
		Detail: string(access)}
	if err := s.write([]Workspace{updated}, nil, nil, event); err != nil {
		return WorkspaceShare{}, err
	}
	s.workspaces[id] = updated
	return share, nil
}

// Unshare は、actor が id のワークスペースの login への共有を取り消す。
func (s *WorkspaceStore) Unshare(id, actor, login string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.workspaces[id]
	if !ok {
		return ErrWorkspaceNotFound
	}
	if _, shared := current.ShareOf(login); !shared {
		return ErrWorkspaceShareNotFound
	}
	updated := cloneWorkspace(current)
	updated.Shares = slices.DeleteFunc(updated.Shares, func(other WorkspaceShare) bool { return other.Login == login })
	event := AuditEvent{RecordedAt: s.now().UTC(), Actor: actor, Action: AuditWorkspaceUnshared, Target: id + ":" + login}
	if err := s.write([]Workspace{updated}, nil, nil, event); err != nil {
		return err
	}
	s.workspaces[id] = updated
	return nil
}

// write は変更を保存先へ書く。journal を持たない store は監査の記録を溜める。呼び出し側が mu を持つ。
func (s *WorkspaceStore) write(
	workspaces []Workspace, removed []string, changes []WorkspaceChange, events ...AuditEvent,
) error {
	if s.journal == nil {
		s.pending = append(s.pending, events...)
		return nil
	}
	if err := s.journal(workspaces, removed, changes, events); err != nil {
		return fmt.Errorf("workspace store: recording the change: %w", err)
	}
	return nil
}

// attach は、メモリの値を journal で書き、以後の変更を journal で書く。既に journal を持つ store では
// 何もしない。書けなかった attach は、次の Settle でやり直す。
func (s *WorkspaceStore) attach(journal workspaceJournal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.journal != nil {
		return nil
	}
	if len(s.workspaces) > 0 || len(s.pending) > 0 {
		workspaces := make([]Workspace, 0, len(s.workspaces))
		var changes []WorkspaceChange
		for _, workspace := range s.workspaces {
			workspaces = append(workspaces, workspace)
			changes = append(changes, s.changes[workspace.Id]...)
		}
		if err := journal(workspaces, nil, changes, s.pending); err != nil {
			return fmt.Errorf("workspace store: recording the workspaces: %w", err)
		}
	}
	s.journal, s.pending = journal, nil
	return nil
}

func checkWorkspaceName(name string) error {
	if strings.TrimSpace(name) == "" || len(name) > 256 {
		return fmt.Errorf("the name must have 1 to 256 bytes: %w", ErrWorkspaceInvalid)
	}
	return nil
}

func cloneWorkspace(workspace Workspace) Workspace {
	workspace.State = slices.Clone(workspace.State)
	workspace.Shares = slices.Clone(workspace.Shares)
	return workspace
}

func cloneChange(change WorkspaceChange) WorkspaceChange {
	change.Fields = slices.Clone(change.Fields)
	return change
}

func newWorkspaceId() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("creating the workspace id: %w", err)
	}
	return "ws-" + hex.EncodeToString(bytes[:]), nil
}
