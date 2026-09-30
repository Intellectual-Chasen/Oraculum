package investigationdb_test

import (
	"context"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/investigationdb"
)

// ワークスペースの表を持たない調査の file にワークスペースを書き、開き直して同じ値で読む。
func TestWorkspacesAreReplacedRemovedAndReadBack(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "investigation")
	db := create(t, dir)
	first := investigationdb.Workspace{Id: "ws-1", Owner: "alice", Name: "初め", State: `{"a":1}`, Revision: 1,
		UpdatedAt: "2026-09-28T00:00:00Z", UpdatedBy: "alice"}
	second := investigationdb.Workspace{Id: "ws-2", Owner: "bob", Name: "b", State: `{}`, Revision: 1,
		UpdatedAt: "2026-09-28T00:01:00Z", UpdatedBy: "bob"}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	// 追加した表を持たない file を開ける。
	db, contents, err := investigationdb.Open(ctx, dir)
	if err != nil || len(contents.Workspaces) != 0 {
		t.Fatalf("workspaces=%+v err=%v", contents.Workspaces, err)
	}
	share := investigationdb.WorkspaceShare{Login: "carol", Access: "view", GrantedBy: "bob", GrantedAt: "2026-09-28T00:02:00Z"}
	second.Shares = []investigationdb.WorkspaceShare{share}
	change := func(id string, revision int64, clientChangeId string) investigationdb.WorkspaceChange {
		return investigationdb.WorkspaceChange{WorkspaceId: id, Revision: revision, ClientChangeId: clientChangeId,
			Actor: "alice", Fields: []string{"a"}, RecordedAt: "2026-09-28T00:02:30Z"}
	}
	if err := db.ReplaceWorkspaces(ctx, []investigationdb.Workspace{first, second}, nil,
		[]investigationdb.WorkspaceChange{change("ws-1", 2, "c-2"), change("ws-2", 2, "c-2")}, nil); err != nil {
		t.Fatal(err)
	}
	first.State, first.Revision = `{"a":2}`, 4
	first.Shares = []investigationdb.WorkspaceShare{share}
	event := investigationdb.AuditEvent{RecordedAt: "2026-09-28T00:03:00Z", Actor: "alice", Action: "workspace_shared",
		Target: "ws-1:carol", Detail: "view"}
	// 変更の記録は 1000 件を超えてもすべて残り、消したワークスペースの変更は残らない。
	first.Changes = []investigationdb.WorkspaceChange{change("ws-1", 2, "c-2")}
	var added []investigationdb.WorkspaceChange
	for revision := int64(3); revision <= 1203; revision++ {
		added = append(added, change("ws-1", revision, "c-"+strconv.FormatInt(revision, 10)))
	}
	first.Changes = append(first.Changes, added...)
	first.Revision = 1203
	if err := db.ReplaceWorkspaces(ctx, []investigationdb.Workspace{first}, []string{"ws-2"}, added,
		[]investigationdb.AuditEvent{event}); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceWorkspaces(ctx, nil, nil, nil, nil); err != nil {
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
	if want := []investigationdb.Workspace{first}; !reflect.DeepEqual(contents.Workspaces, want) {
		t.Fatalf("workspaces=%+v want=%+v", contents.Workspaces, want)
	}
	if events, err := db.AuditEvents(ctx); err != nil || !reflect.DeepEqual(events, []investigationdb.AuditEvent{event}) {
		t.Fatalf("events=%+v err=%v", events, err)
	}
}
