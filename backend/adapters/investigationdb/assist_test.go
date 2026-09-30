package investigationdb_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/investigationdb"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func samplePermissions() []core.AssistPermissionRevision {
	return []core.AssistPermissionRevision{
		{Provider: core.AssistProviderClaude, RevisionNumber: 1, Action: core.AssistPermissionActionGrant,
			Analyst: "analyst-a", RecordedAt: recordedAt(10)},
		{Provider: core.AssistProviderClaude, RevisionNumber: 2, Action: core.AssistPermissionActionRevoke,
			Analyst: "analyst-b", RecordedAt: recordedAt(11)},
	}
}

// countTables は path の file が持つ、name の表の数を返す。
func countTables(t *testing.T, path, name string) int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = ?`,
		name).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// AI 支援の表を持たない調査の file を開き、記録が無いものとして読む。最初の書き込みが表を作り、
// 開き直した後も改訂を記録した順に読み戻す。
func TestAssistPermissionsOnAFileWithoutTheAssistTables(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, investigationdb.FileName)
	if err := create(t, dir).Close(); err != nil {
		t.Fatal(err)
	}
	if countTables(t, path, "assist_permission_revision") != 0 {
		t.Fatal("the created file already holds the assist table")
	}
	db, contents, err := investigationdb.Open(ctx, dir)
	if err != nil {
		t.Fatalf("opening a file without the assist tables: %v", err)
	}
	if len(contents.AssistPermissions) != 0 {
		t.Errorf("permissions = %+v, want none", contents.AssistPermissions)
	}
	for _, revision := range samplePermissions() {
		if err := db.InsertAssistPermission(ctx, revision); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if countTables(t, path, "assist_permission_revision") != 1 {
		t.Fatal("the first write did not create the assist table")
	}
	reopened, contents, err := investigationdb.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reflect.DeepEqual(contents.AssistPermissions, samplePermissions()) {
		t.Errorf("permissions = %+v, want %+v", contents.AssistPermissions, samplePermissions())
	}
	// 既存の値は AI 支援の表を足した後も同じに読める。
	if !reflect.DeepEqual(contents.Sources, sampleSources()) {
		t.Errorf("sources = %+v, want %+v", contents.Sources, sampleSources())
	}
}

// 同じ提供者の同じ改訂の番号を 2 回書かない。
func TestAssistPermissionRevisionNumberIsUniquePerProvider(t *testing.T) {
	ctx := context.Background()
	db := create(t, t.TempDir())
	defer db.Close()
	revision := samplePermissions()[0]
	if err := db.InsertAssistPermission(ctx, revision); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertAssistPermission(ctx, revision); err == nil {
		t.Fatal("the same revision number was recorded twice")
	}
}

// core の Validate を通らない改訂を持つ file は開かない。
func TestOpenRefusesAnInvalidAssistPermission(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db := create(t, dir)
	if err := db.InsertAssistPermission(ctx, samplePermissions()[0]); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	execRaw(t, filepath.Join(dir, investigationdb.FileName), `UPDATE assist_permission_revision SET analyst = ''`)
	if _, _, err := investigationdb.Open(ctx, dir); err == nil {
		t.Fatal("opening a file with an invalid assist permission succeeded")
	}
}
