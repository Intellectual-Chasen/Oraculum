package pipeline

import (
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWorkspaceStoreDetectsAStaleRevision(t *testing.T) {
	store := NewWorkspaceStore(nil)
	created, err := store.Create("alice", "調査の初め", []byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if created.Revision != 1 || created.Owner != "alice" || created.UpdatedBy != "alice" {
		t.Fatalf("created=%+v", created)
	}
	updated, change, err := store.Update(created.Id, "alice", 1, "調査の続き", []byte(`{"a":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || string(updated.State) != `{"a":2}` || change.Revision != 2 ||
		len(change.Fields) != 1 || change.Fields[0] != WorkspaceAllFields {
		t.Fatalf("updated=%+v change=%+v", updated, change)
	}
	current, _, err := store.Update(created.Id, "alice", 1, "古い画面", []byte(`{"a":3}`))
	if !errors.Is(err, ErrWorkspaceRevisionConflict) || current.Revision != 2 {
		t.Fatalf("current=%+v err=%v", current, err)
	}
	if stored, _ := store.Get(created.Id); string(stored.State) != `{"a":2}` {
		t.Fatalf("the stale update replaced the state: %s", stored.State)
	}
}

func TestWorkspaceStoreListsTheOwnersWorkspacesNewestFirst(t *testing.T) {
	store := NewWorkspaceStore(nil)
	clock := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { clock = clock.Add(time.Minute); return clock }
	first, _ := store.Create("alice", "first", []byte(`{}`))
	second, _ := store.Create("alice", "second", []byte(`{}`))
	if _, err := store.Create("bob", "bob's", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	listed := store.List(func(workspace Workspace) bool { return workspace.Owner == "alice" })
	if len(listed) != 2 || listed[0].Id != second.Id || listed[1].Id != first.Id {
		t.Fatalf("listed=%+v", listed)
	}
}

func TestWorkspaceStoreRejectsBlankNamesAndHoldsAnyNumberOfWorkspaces(t *testing.T) {
	store := NewWorkspaceStore(nil)
	if _, err := store.Create("alice", " ", []byte(`{}`)); !errors.Is(err, ErrWorkspaceInvalid) {
		t.Fatalf("blank name err=%v", err)
	}
	const count = 150
	for range count {
		if _, err := store.Create("alice", "w", []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	if listed := store.List(func(workspace Workspace) bool { return workspace.Owner == "alice" }); len(listed) != count {
		t.Fatalf("listed=%d", len(listed))
	}
}

// 名前を持たない作成は、owner の「ワークスペース N」の最大の N に 1 を足した名前を付ける。
func TestWorkspaceStoreNamesUnnamedWorkspacesAfterTheLargestNumber(t *testing.T) {
	store := NewWorkspaceStore(nil)
	for _, name := range []string{"ワークスペース 2", "ワークスペース 7", "ワークスペース x", "メモ 9"} {
		if _, err := store.Create("alice", name, []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Create("bob", "ワークスペース 40", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	first, err := store.Create("alice", "", []byte(`{}`))
	if err != nil || first.Name != "ワークスペース 8" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, _ := store.Create("alice", "", []byte(`{}`))
	fresh, _ := store.Create("carol", "", []byte(`{}`))
	if second.Name != "ワークスペース 9" || fresh.Name != "ワークスペース 1" {
		t.Fatalf("second=%q fresh=%q", second.Name, fresh.Name)
	}
	// 同時に作っても名前は重ならない。
	var wg sync.WaitGroup
	names := make(chan string, 20)
	for range 20 {
		wg.Go(func() {
			created, err := store.Create("dave", "", []byte(`{}`))
			if err != nil {
				t.Error(err)
			}
			names <- created.Name
		})
	}
	wg.Wait()
	close(names)
	var got, want []string
	for name := range names {
		got = append(got, name)
	}
	for number := 1; number <= 20; number++ {
		want = append(want, "ワークスペース "+strconv.Itoa(number))
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("names=%v", got)
	}
	// 数字だけからなる番号だけを数え、int の最大値の番号は数えない。
	for _, name := range []string{"ワークスペース +50", "ワークスペース 9223372036854775807", "ワークスペース 1e3"} {
		if _, err := store.Create("erin", name, []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	if next, _ := store.Create("erin", "", []byte(`{}`)); next.Name != "ワークスペース 1" {
		t.Fatalf("next=%q", next.Name)
	}
}

func TestWorkspaceStoreWritesBeforeKeepingAndFlushesOnAttach(t *testing.T) {
	store := NewWorkspaceStore(nil)
	created, err := store.Create("alice", "w", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var flushed []Workspace
	if _, err := store.Share(created.Id, "alice", "bob", WorkspaceView); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Change(created.Id, "alice", "c-1", 1, map[string]json.RawMessage{"a": []byte(`1`)}); err != nil {
		t.Fatal(err)
	}
	var flushedEvents []AuditEvent
	var flushedChanges []WorkspaceChange
	if err := store.attach(func(workspaces []Workspace, _ []string, changes []WorkspaceChange, events []AuditEvent) error {
		flushed, flushedChanges, flushedEvents = workspaces, changes, events
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(flushed) != 1 || flushed[0].Id != created.Id || len(flushed[0].Shares) != 1 {
		t.Fatalf("flushed=%+v", flushed)
	}
	if len(flushedChanges) != 1 || flushedChanges[0].ClientChangeId != "c-1" || flushedChanges[0].Revision != 2 {
		t.Fatalf("changes=%+v", flushedChanges)
	}
	if len(flushedEvents) != 1 || flushedEvents[0].Action != AuditWorkspaceShared ||
		flushedEvents[0].Target != created.Id+":bob" || flushedEvents[0].Detail != "view" {
		t.Fatalf("events=%+v", flushedEvents)
	}
	store.journal = func([]Workspace, []string, []WorkspaceChange, []AuditEvent) error { return errors.New("disk full") }
	if _, _, err := store.Update(created.Id, "alice", 2, "renamed", []byte(`{}`)); err == nil {
		t.Fatal("the unwritten update was accepted")
	}
	if _, err := store.Change(created.Id, "alice", "c-2", 2, map[string]json.RawMessage{"b": []byte(`1`)}); err == nil {
		t.Fatal("the unwritten change was accepted")
	}
	if _, err := store.Change(created.Id, "alice", "c-2", 2, map[string]json.RawMessage{"b": []byte(`1`)}); err == nil {
		t.Fatal("the unwritten change was recorded as applied")
	}
	if _, err := store.Share(created.Id, "alice", "carol", WorkspaceEdit); err == nil {
		t.Fatal("the unwritten share was accepted")
	}
	if err := store.Unshare(created.Id, "alice", "bob"); err == nil {
		t.Fatal("the unwritten unshare was accepted")
	}
	if stored, _ := store.Get(created.Id); len(stored.Shares) != 1 || stored.Shares[0].Login != "bob" {
		t.Fatalf("shares=%+v", stored.Shares)
	}
	if err := store.Delete(created.Id, "alice"); err == nil {
		t.Fatal("the unwritten deletion was accepted")
	}
	if stored, ok := store.Get(created.Id); !ok || stored.Name != "w" {
		t.Fatalf("stored=%+v ok=%v", stored, ok)
	}
}

// 書けなかった attach は、次の attach で溜めた値を書く。書けた後の attach は何もしない。
func TestWorkspaceStoreRetriesAFailedAttach(t *testing.T) {
	store := NewWorkspaceStore(nil)
	if _, err := store.Create("alice", "w", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := store.attach(func([]Workspace, []string, []WorkspaceChange, []AuditEvent) error {
		return errors.New("disk full")
	}); err == nil {
		t.Fatal("the failed attach was accepted")
	}
	writes := 0
	journal := func(workspaces []Workspace, _ []string, _ []WorkspaceChange, _ []AuditEvent) error {
		writes++
		if len(workspaces) != 1 {
			t.Errorf("workspaces=%v", workspaces)
		}
		return nil
	}
	for range 2 {
		if err := store.attach(journal); err != nil {
			t.Fatal(err)
		}
	}
	if writes != 1 {
		t.Fatalf("writes=%d", writes)
	}
}

func patchOf(values map[string]string) map[string]json.RawMessage {
	patch := map[string]json.RawMessage{}
	for field, value := range values {
		patch[field] = json.RawMessage(value)
	}
	return patch
}

// 別の欄の同時の変更は両方が残り、同じ欄の変更は重なった欄の競合になる。
func TestWorkspaceStoreChangesFieldsAndDetectsOverlappingFields(t *testing.T) {
	store := NewWorkspaceStore(nil)
	created, err := store.Create("alice", "w", []byte(`{"a":1,"b":1,"c":1}`))
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Change(created.Id, "alice", "c-a", 1, patchOf(map[string]string{"a": `2`}))
	if err != nil || first.Workspace.Revision != 2 || first.Replayed {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := store.Change(created.Id, "bob", "c-b", 1, patchOf(map[string]string{"b": `{"x":[1]}`}))
	if err != nil || string(second.Workspace.State) != `{"a":2,"b":{"x":[1]},"c":1}` {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	// 値が null の欄は state から消える。
	removed, err := store.Change(created.Id, "bob", "c-null", 3, patchOf(map[string]string{"c": `null`}))
	if err != nil || string(removed.Workspace.State) != `{"a":2,"b":{"x":[1]}}` {
		t.Fatalf("removed=%+v err=%v", removed, err)
	}
	_, err = store.Change(created.Id, "carol", "c-ac", 1, patchOf(map[string]string{"a": `3`, "c": `3`, "d": `3`}))
	var conflict *WorkspaceConflictError
	if !errors.As(err, &conflict) || !errors.Is(err, ErrWorkspaceRevisionConflict) ||
		!slices.Equal(conflict.Fields, []string{"a", "c"}) || conflict.Current.Revision != 4 {
		t.Fatalf("err=%v", err)
	}
	if stored, _ := store.Get(created.Id); string(stored.State) != `{"a":2,"b":{"x":[1]}}` {
		t.Fatalf("the conflicting change was applied: %s", stored.State)
	}
	// 全体の置き換えの後は、どの欄も競合する。
	if _, _, err := store.Update(created.Id, "alice", 4, "w", []byte(`{"a":9}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Change(created.Id, "bob", "c-z", 4, patchOf(map[string]string{"z": `1`})); !errors.As(err, &conflict) ||
		!slices.Equal(conflict.Fields, []string{WorkspaceAllFields}) {
		t.Fatalf("err=%v", err)
	}
	for _, patch := range []map[string]json.RawMessage{nil, patchOf(map[string]string{"*": `1`})} {
		if _, err := store.Change(created.Id, "alice", "c-bad", 5, patch); !errors.Is(err, ErrWorkspaceInvalid) {
			t.Errorf("patch=%.40v err=%v", patch, err)
		}
	}
}

// 変更を足した後の state が 1 MiB を超えても記録する。
func TestWorkspaceStoreChangesAStateLargerThanOneMebibyte(t *testing.T) {
	store := NewWorkspaceStore(nil)
	created, err := store.Create("alice", "w", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	large := `"` + strings.Repeat("x", 3<<20) + `"`
	result, err := store.Change(created.Id, "alice", "c-large", 1, patchOf(map[string]string{"a": large}))
	if err != nil {
		t.Fatal(err)
	}
	if stored, _ := store.Get(created.Id); string(stored.State) != `{"a":`+large+`}` || result.Workspace.Revision != 2 {
		t.Fatalf("state bytes=%d revision=%d", len(stored.State), result.Workspace.Revision)
	}
}

// 同じ clientChangeId の再送は、二重に適用せず前の結果を返す。
func TestWorkspaceStoreReplaysTheSameClientChange(t *testing.T) {
	store := NewWorkspaceStore(nil)
	created, err := store.Create("alice", "w", []byte(`{"n":0}`))
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Change(created.Id, "alice", "c-1", 1, patchOf(map[string]string{"n": `1`}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Change(created.Id, "bob", "c-2", 2, patchOf(map[string]string{"m": `1`})); err != nil {
		t.Fatal(err)
	}
	again, err := store.Change(created.Id, "alice", "c-1", 1, patchOf(map[string]string{"n": `1`}))
	if err != nil || !again.Replayed || again.Change.Revision != first.Change.Revision || again.Workspace.Revision != 3 {
		t.Fatalf("again=%+v err=%v", again, err)
	}
	// 別の利用者は、ほかの利用者の clientChangeId の結果を受け取れない。
	if _, err := store.Change(created.Id, "bob", "c-1", 1, patchOf(map[string]string{"n": `1`})); !errors.Is(err, ErrWorkspaceInvalid) {
		t.Fatalf("err=%v", err)
	}
}

// 変更の記録をすべて保ち、最初の revision を base にした変更でも変えた欄を判定する。
func TestWorkspaceStoreKeepsEveryChange(t *testing.T) {
	store := NewWorkspaceStore(nil)
	var journaled []WorkspaceChange
	store.journal = func(_ []Workspace, _ []string, changes []WorkspaceChange, _ []AuditEvent) error {
		journaled = append(journaled, changes...)
		return nil
	}
	created, err := store.Create("alice", "w", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	const count = 1500
	for revision := int64(1); revision <= count; revision++ {
		if _, err := store.Change(created.Id, "alice", "c-"+strconv.FormatInt(revision, 10), revision,
			patchOf(map[string]string{"a": `1`})); err != nil {
			t.Fatal(err)
		}
	}
	if len(store.changes[created.Id]) != count || len(journaled) != count {
		t.Fatalf("kept=%d journaled=%d", len(store.changes[created.Id]), len(journaled))
	}
	var conflict *WorkspaceConflictError
	if _, err := store.Change(created.Id, "bob", "old-a", 1, patchOf(map[string]string{"a": `2`})); !errors.As(err, &conflict) ||
		!slices.Equal(conflict.Fields, []string{"a"}) {
		t.Fatalf("err=%v", err)
	}
	if _, err := store.Change(created.Id, "bob", "old-b", 1, patchOf(map[string]string{"b": `1`})); err != nil {
		t.Fatalf("the first base err=%v", err)
	}
	// 最初の変更の clientChangeId の再送は、前の結果を返す。
	if again, err := store.Change(created.Id, "alice", "c-1", 1, patchOf(map[string]string{"a": `1`})); err != nil ||
		!again.Replayed {
		t.Fatalf("again=%+v err=%v", again, err)
	}
}
