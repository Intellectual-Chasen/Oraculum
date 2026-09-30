package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/investigationdb"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// 調査の file を作る前と後に保存したワークスペースと共有は、開き直した後も読め、共有の監査の記録が
// 残る。
func TestWorkspacesSavedAroundTheInvestigationCreationAreKept(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "investigation")
	opened := openInvestigation(t, investigationFlag, dir, sourceRootFlag, runFixtureDir)
	before, err := opened.investigation.Workspaces().Create("alice", "作る前", []byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	// file を作る前の共有と監査の記録は、Settle の attach で file へ書く。
	if _, err := opened.investigation.Workspaces().Share(before.Id, "alice", "bob", pipeline.WorkspaceView); err != nil {
		t.Fatal(err)
	}
	if _, err := opened.investigation.Workspaces().Change(before.Id, "alice", "c-before", before.Revision,
		map[string]json.RawMessage{"a": json.RawMessage(`5`)}); err != nil {
		t.Fatal(err)
	}
	if err := opened.stages.StartRequestedLoading([]pipeline.SourcePlan{
		{OriginPath: "squid.log", FormatKey: "squid_combined"},
	}); err != nil {
		t.Fatal(err)
	}
	opened.stages.Wait()
	if _, loaded := opened.stages.Loaded(); !loaded {
		t.Fatalf("the loading did not complete: %+v", opened.stages.Snapshot().Loading)
	}
	after, _, err := opened.investigation.Workspaces().Update(before.Id, "alice", before.Revision+1, "作った後",
		[]byte(`{"a":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opened.investigation.Workspaces().Change(before.Id, "alice", "c-after", after.Revision,
		map[string]json.RawMessage{"b": json.RawMessage(`1`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := opened.investigation.Workspaces().Share(before.Id, "alice", "carol", pipeline.WorkspaceEdit); err != nil {
		t.Fatal(err)
	}
	if err := opened.investigation.Workspaces().Unshare(before.Id, "alice", "bob"); err != nil {
		t.Fatal(err)
	}
	after, _ = opened.investigation.Workspaces().Get(before.Id)
	opened.close(t)

	reopened := openInvestigation(t, investigationFlag, dir)
	got, ok := reopened.investigation.Workspaces().Get(before.Id)
	if !ok || got.Name != "作った後" || string(got.State) != `{"a":2,"b":1}` || got.Revision != after.Revision ||
		!got.UpdatedAt.Equal(after.UpdatedAt) || len(got.Shares) != 1 || got.Shares[0].Login != "carol" ||
		got.Shares[0].Access != pipeline.WorkspaceEdit || !got.Shares[0].GrantedAt.Equal(after.Shares[0].GrantedAt) {
		t.Fatalf("got=%+v ok=%v want=%+v", got, ok, after)
	}
	// file を作る前と後の変更の記録が残り、同じ clientChangeId の再送は開き直した後も二重に適用しない。
	for _, clientChangeId := range []string{"c-before", "c-after"} {
		result, err := reopened.investigation.Workspaces().Change(before.Id, "alice", clientChangeId, 1,
			map[string]json.RawMessage{"z": json.RawMessage(`1`)})
		if err != nil || !result.Replayed || result.Workspace.Revision != after.Revision {
			t.Fatalf("%s result=%+v err=%v", clientChangeId, result, err)
		}
	}
	// 共有中のワークスペースを消すと、共有も消え、取り消しを監査に記録する。
	if err := reopened.investigation.Workspaces().Delete(before.Id, "alice"); err != nil {
		t.Fatal(err)
	}
	reopened.close(t)
	again := openInvestigation(t, investigationFlag, dir)
	if _, ok := again.investigation.Workspaces().Get(before.Id); ok {
		t.Fatal("the deleted workspace is read")
	}
	again.close(t)

	db, _, err := investigationdb.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	events, err := db.AuditEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var shareEvents []string
	for _, event := range events {
		if strings.HasPrefix(event.Action, "workspace_") {
			shareEvents = append(shareEvents, event.Actor+" "+event.Action+" "+event.Target+" "+event.Detail)
		}
	}
	want := []string{
		"alice workspace_shared " + before.Id + ":bob view",
		"alice workspace_shared " + before.Id + ":carol edit",
		"alice workspace_unshared " + before.Id + ":bob ",
		"alice workspace_unshared " + before.Id + ":carol workspace_deleted",
	}
	if !slices.Equal(shareEvents, want) {
		t.Fatalf("events=%q want=%q", shareEvents, want)
	}
}
