package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/accountsdb"
)

func runAccountsCommand(t *testing.T, stdin string, args ...string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runAccounts(args, strings.NewReader(stdin), &stdout, &stderr)
	return code, stdout.String() + stderr.String()
}

// 運用者は、パスワードを標準入力から渡してアカウントを足し、無効にし、パスワードを変えられる。
func TestAccountsCommandManagesAnAccount(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "accounts.sqlite")
	if code, out := runAccountsCommand(t, "correct horse\n", "add", "--accounts", path, "alice",
		"--display-name", "石橋"); code != 0 || strings.Contains(out, "correct horse") {
		t.Fatalf("add code=%d out=%q", code, out)
	}
	db, err := accountsdb.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	session, err := db.Login(ctx, "alice", "correct horse")
	if err != nil || session.Account.DisplayName != "石橋" {
		t.Fatalf("session=%+v err=%v", session, err)
	}
	if code, out := runAccountsCommand(t, "", "disable", "--accounts", path, "alice"); code != 0 {
		t.Fatalf("disable code=%d out=%q", code, out)
	}
	if _, err := db.SessionAccount(ctx, session.Token); !errors.Is(err, accountsdb.ErrSessionInvalid) {
		t.Fatalf("the session survives the disabling: err=%v", err)
	}
	if code, out := runAccountsCommand(t, "", "enable", "--accounts", path, "alice"); code != 0 {
		t.Fatalf("enable code=%d out=%q", code, out)
	}
	if code, out := runAccountsCommand(t, "another horse\r\n", "passwd", "--accounts", path, "alice"); code != 0 {
		t.Fatalf("passwd code=%d out=%q", code, out)
	}
	if _, err := db.Login(ctx, "alice", "another horse"); err != nil {
		t.Fatalf("login with the new password: %v", err)
	}
}

func TestAccountsCommandRejectsInvalidArguments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.sqlite")
	for _, args := range [][]string{
		nil,
		{"remove", "--accounts", path, "alice"},
		{"add", "alice"},
		{"add", "--accounts", path},
		{"add", "--accounts", path, "alice", "bob"},
		{"disable", "--accounts", path, "alice", "--display-name", "x"},
		{"add", "--accounts", path, "local"},
	} {
		if code, _ := runAccountsCommand(t, "correct horse\n", args...); code == 0 {
			t.Errorf("args=%q accepted", args)
		}
	}
	// パスワードの行が空なら足さない。
	if code, _ := runAccountsCommand(t, "\n", "add", "--accounts", path, "alice"); code == 0 {
		t.Error("an empty password accepted")
	}
	// file の無い disable は file を作らない。
	missing := filepath.Join(t.TempDir(), "missing.sqlite")
	if code, _ := runAccountsCommand(t, "", "disable", "--accounts", missing, "alice"); code == 0 {
		t.Error("disable on a missing file accepted")
	}
}
