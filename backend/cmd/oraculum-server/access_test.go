package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/accountsdb"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

func accountsWith(t *testing.T, logins ...string) accountsPort {
	t.Helper()
	db, err := accountsdb.Create(context.Background(), filepath.Join(t.TempDir(), "accounts.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, login := range logins {
		if err := db.AddAccount(context.Background(), login, login, "correct horse"); err != nil {
			t.Fatal(err)
		}
	}
	return accountsPort{db: db}
}

// 調査の file を作る前に --admin で与えた役割は、読み込みで作った調査に残り、開き直した後も読める。
func TestAdminGrantedBeforeTheInvestigationExistsIsKept(t *testing.T) {
	ctx := context.Background()
	accounts := accountsWith(t, "alice")
	dir := filepath.Join(t.TempDir(), "investigation")
	opened := openInvestigation(t, investigationFlag, dir, sourceRootFlag, runFixtureDir)
	if _, err := prepareAccess(ctx, "alice", accounts, opened.investigation); err != nil {
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
	opened.close(t)

	reopened := openInvestigation(t, investigationFlag, dir)
	defer reopened.close(t)
	if role, ok := reopened.investigation.Access().Role("alice"); !ok || role != pipeline.RoleAdmin {
		t.Fatalf("role=%q ok=%v", role, ok)
	}
	// 役割を持つ調査は、--admin 無しでも開ける。
	if _, err := prepareAccess(ctx, "", accounts, reopened.investigation); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareAccessRefusesAnInvestigationWithoutMembers(t *testing.T) {
	ctx := context.Background()
	accounts := accountsWith(t, "alice")
	if _, err := prepareAccess(ctx, "", accounts, nil); err == nil {
		t.Fatal("started without any member")
	}
	if _, err := prepareAccess(ctx, "nobody", accounts, nil); err == nil {
		t.Fatal("made a missing account the admin")
	}
	access, err := prepareAccess(ctx, "alice", accounts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if role, ok := access.Role("alice"); !ok || role != pipeline.RoleAdmin {
		t.Fatalf("role=%q ok=%v", role, ok)
	}
}

func TestParseArgsRequiresAccountsForTheAdmin(t *testing.T) {
	if _, err := parseArgs([]string{"squid_combined:logs/access.log", adminFlag, "alice"}); err == nil {
		t.Fatal("--admin without --accounts accepted")
	}
	opts, err := parseArgs([]string{"squid_combined:logs/access.log", adminFlag, "alice", accountsFlag, "a.sqlite"})
	if err != nil || opts.admin != "alice" {
		t.Fatalf("opts=%+v err=%v", opts, err)
	}
}
