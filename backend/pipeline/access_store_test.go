package pipeline

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestRoleIncludesTheOperationsOfLowerRoles(t *testing.T) {
	for _, check := range []struct {
		role, required Role
		want           bool
	}{
		{RoleAdmin, RoleEditor, true}, {RoleAdmin, RoleViewer, true}, {RoleEditor, RoleViewer, true},
		{RoleEditor, RoleEditor, true}, {RoleViewer, RoleEditor, false}, {RoleEditor, RoleAdmin, false},
		{Role("owner"), RoleViewer, false},
	} {
		if got := check.role.Includes(check.required); got != check.want {
			t.Errorf("%s.Includes(%s)=%v want=%v", check.role, check.required, got, check.want)
		}
	}
}

func TestAccessStoreKeepsOneAdminAndRecordsEachChange(t *testing.T) {
	store := NewAccessStore(nil)
	clock := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return clock }
	if _, err := store.Grant(OperatorActor, "alice", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Grant("alice", "bob", RoleViewer); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Grant("alice", "bob", Role("owner")); !errors.Is(err, ErrUnknownRole) {
		t.Fatalf("unknown role err=%v", err)
	}
	if _, err := store.Grant("alice", "alice", RoleEditor); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demoting the last admin err=%v", err)
	}
	if err := store.Revoke("alice", "alice"); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("revoking the last admin err=%v", err)
	}
	if err := store.Revoke("alice", "carol"); !errors.Is(err, ErrMemberNotFound) {
		t.Fatalf("revoking a stranger err=%v", err)
	}
	if err := store.Revoke("alice", "bob"); err != nil {
		t.Fatal(err)
	}
	if role, ok := store.Role("bob"); ok {
		t.Fatalf("bob keeps %s", role)
	}
	if role, ok := store.Role("alice"); !ok || role != RoleAdmin {
		t.Fatalf("alice role=%s ok=%v", role, ok)
	}

	// 調査の file を作る時点で、溜めた役割と監査の記録を書く。
	var written [][]AuditEvent
	var writtenMembers []Member
	if err := store.attach(func(members []Member, _ []string, events []AuditEvent) error {
		writtenMembers, written = members, append(written, events)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := []AuditEvent{
		{RecordedAt: clock, Actor: OperatorActor, Action: AuditMemberGranted, Target: "alice", Detail: "admin"},
		{RecordedAt: clock, Actor: "alice", Action: AuditMemberGranted, Target: "bob", Detail: "viewer"},
		{RecordedAt: clock, Actor: "alice", Action: AuditMemberRevoked, Target: "bob"},
	}
	if len(written) != 1 || !reflect.DeepEqual(written[0], want) {
		t.Fatalf("written=%+v want=%+v", written, want)
	}
	if len(writtenMembers) != 1 || writtenMembers[0].Login != "alice" {
		t.Fatalf("members=%+v", writtenMembers)
	}
}

// 書けなかった attach は、次の attach で溜めた値を書く。書けた後の attach は何もしない。
func TestAccessStoreRetriesAFailedAttach(t *testing.T) {
	store := NewAccessStore(nil)
	if _, err := store.Grant(OperatorActor, "alice", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := store.attach(func([]Member, []string, []AuditEvent) error { return errors.New("disk full") }); err == nil {
		t.Fatal("the failed attach was accepted")
	}
	writes := 0
	journal := func(members []Member, _ []string, events []AuditEvent) error {
		writes++
		if len(members) != 1 || len(events) != 1 {
			t.Errorf("members=%v events=%v", members, events)
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

// 管理者が 2 人いれば、片方を降格し、外せる。
func TestAccessStoreChangesAnAdminWhileAnotherRemains(t *testing.T) {
	store := NewAccessStore([]Member{{Login: "alice", Role: RoleAdmin}, {Login: "bob", Role: RoleAdmin}})
	if _, err := store.Grant("alice", "bob", RoleEditor); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Grant("alice", "bob", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke("bob", "alice"); err != nil {
		t.Fatal(err)
	}
	if member, ok := store.Member("bob"); !ok || member.Role != RoleAdmin {
		t.Fatalf("bob=%+v ok=%v", member, ok)
	}
}

// 保存先へ書けない変更はメモリに確定しない。
func TestAccessStoreDoesNotKeepAChangeItCouldNotWrite(t *testing.T) {
	store := NewAccessStore([]Member{{Login: "alice", Role: RoleAdmin}})
	store.journal = func([]Member, []string, []AuditEvent) error { return errors.New("disk full") }
	if _, err := store.Grant("alice", "bob", RoleEditor); err == nil {
		t.Fatal("the change was accepted")
	}
	if _, ok := store.Role("bob"); ok {
		t.Fatal("the unwritten change is kept")
	}
}
