package investigationdb_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/investigationdb"
)

// 役割の表を持たない調査の file は、役割が空の調査として開け、最初の書き込みで表を作る。
func TestMembersAreAddedToAnInvestigationWithoutTheTable(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "investigation")
	db := create(t, dir)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, contents, err := investigationdb.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents.Members) != 0 {
		t.Fatalf("members=%v", contents.Members)
	}
	if events, err := db.AuditEvents(ctx); err != nil || len(events) != 0 {
		t.Fatalf("events=%v err=%v", events, err)
	}
	alice := investigationdb.Member{Login: "alice", Role: "admin", GrantedBy: "--admin", GrantedAt: "2026-09-28T00:00:00Z"}
	bob := investigationdb.Member{Login: "bob", Role: "viewer", GrantedBy: "alice", GrantedAt: "2026-09-28T00:01:00Z"}
	granted := investigationdb.AuditEvent{RecordedAt: "2026-09-28T00:01:00Z", Actor: "alice", Action: "member_granted",
		Target: "bob", Detail: "viewer"}
	if err := db.ReplaceMembers(ctx, []investigationdb.Member{alice, bob}, nil, []investigationdb.AuditEvent{granted}); err != nil {
		t.Fatal(err)
	}
	bob.Role = "editor"
	revoked := investigationdb.AuditEvent{RecordedAt: "2026-09-28T00:02:00Z", Actor: "alice", Action: "member_revoked",
		Target: "carol", Detail: ""}
	if err := db.ReplaceMembers(ctx, []investigationdb.Member{bob}, []string{"carol"},
		[]investigationdb.AuditEvent{revoked}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, contents, err = investigationdb.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if want := []investigationdb.Member{alice, bob}; !reflect.DeepEqual(contents.Members, want) {
		t.Fatalf("members=%+v want=%+v", contents.Members, want)
	}
	events, err := db.AuditEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []investigationdb.AuditEvent{granted, revoked}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%+v want=%+v", events, want)
	}
}
