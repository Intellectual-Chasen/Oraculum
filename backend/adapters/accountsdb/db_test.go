// in-package test: hash の反復回数と時計を差し替える。
package accountsdb

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	// 反復回数は hash の強さだけを変え、確かめる手順を変えない。test の所要を短くする。
	hashIterations = 1000
	os.Exit(m.Run())
}

func newAccounts(t *testing.T) (*DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "accounts.sqlite")
	db, err := Create(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

func TestLoginCreatesASessionThatIdentifiesTheAccount(t *testing.T) {
	ctx := context.Background()
	db, _ := newAccounts(t)
	if err := db.AddAccount(ctx, "alice", "石橋", "correct horse"); err != nil {
		t.Fatal(err)
	}
	session, err := db.Login(ctx, "alice", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if session.Account != (Account{Login: "alice", DisplayName: "石橋"}) || len(session.Token) != 43 {
		t.Fatalf("session=%+v", session)
	}
	account, err := db.SessionAccount(ctx, session.Token)
	if err != nil || account.Login != "alice" {
		t.Fatalf("account=%+v err=%v", account, err)
	}
	// file は token そのものを持たない。
	var stored int
	if err := db.db.QueryRow(`SELECT count(*) FROM session WHERE token_sha256 = ?`, session.Token).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("the raw token is stored: count=%d err=%v", stored, err)
	}
}

func TestTwoSessionsIdentifyTwoAccounts(t *testing.T) {
	ctx := context.Background()
	db, _ := newAccounts(t)
	for _, login := range []string{"alice", "bob"} {
		if err := db.AddAccount(ctx, login, login, "password-"+login); err != nil {
			t.Fatal(err)
		}
	}
	alice, err := db.Login(ctx, "alice", "password-alice")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := db.Login(ctx, "bob", "password-bob")
	if err != nil {
		t.Fatal(err)
	}
	for token, want := range map[string]string{alice.Token: "alice", bob.Token: "bob"} {
		if account, err := db.SessionAccount(ctx, token); err != nil || account.Login != want {
			t.Fatalf("account=%+v err=%v want=%s", account, err, want)
		}
	}
}

func TestLoginRejectsWrongCredentialsAndDisabledAccounts(t *testing.T) {
	ctx := context.Background()
	db, _ := newAccounts(t)
	if err := db.AddAccount(ctx, "alice", "alice", "correct horse"); err != nil {
		t.Fatal(err)
	}
	for _, attempt := range [][2]string{{"alice", "wrong password"}, {"nobody", "correct horse"}, {"ALICE", "correct horse"}} {
		if _, err := db.Login(ctx, attempt[0], attempt[1]); !errors.Is(err, ErrLoginRejected) {
			t.Fatalf("login %q err=%v", attempt, err)
		}
	}
	if err := db.SetDisabled(ctx, "alice", true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Login(ctx, "alice", "correct horse"); !errors.Is(err, ErrLoginRejected) {
		t.Fatalf("disabled login err=%v", err)
	}
	if err := db.SetDisabled(ctx, "alice", false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Login(ctx, "alice", "correct horse"); err != nil {
		t.Fatalf("enabled login err=%v", err)
	}
}

func TestSessionsEndByLogoutExpiryPasswordChangeAndDisabling(t *testing.T) {
	ctx := context.Background()
	for name, end := range map[string]func(db *DB, token string, clock *time.Time) error{
		"logout": func(db *DB, token string, _ *time.Time) error { return db.Logout(ctx, token) },
		"expiry": func(_ *DB, _ string, clock *time.Time) error {
			*clock = clock.Add(SessionLifetime)
			return nil
		},
		"password change": func(db *DB, _ string, _ *time.Time) error {
			return db.SetPassword(ctx, "alice", "another horse")
		},
		"disabling": func(db *DB, _ string, _ *time.Time) error { return db.SetDisabled(ctx, "alice", true) },
	} {
		t.Run(name, func(t *testing.T) {
			db, _ := newAccounts(t)
			clock := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
			db.now = func() time.Time { return clock }
			if err := db.AddAccount(ctx, "alice", "alice", "correct horse"); err != nil {
				t.Fatal(err)
			}
			session, err := db.Login(ctx, "alice", "correct horse")
			if err != nil {
				t.Fatal(err)
			}
			if err := end(db, session.Token, &clock); err != nil {
				t.Fatal(err)
			}
			if _, err := db.SessionAccount(ctx, session.Token); !errors.Is(err, ErrSessionInvalid) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

// server が file を開いている間に、運用者の command が別の接続で利用者を無効にできる。
func TestDisablingThroughAnotherConnectionEndsTheSession(t *testing.T) {
	ctx := context.Background()
	server, path := newAccounts(t)
	if err := server.AddAccount(ctx, "alice", "alice", "correct horse"); err != nil {
		t.Fatal(err)
	}
	session, err := server.Login(ctx, "alice", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := operator.SetDisabled(ctx, "alice", true); err != nil {
		t.Fatal(err)
	}
	if err := operator.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := server.SessionAccount(ctx, session.Token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestAddAccountRejectsInvalidValues(t *testing.T) {
	ctx := context.Background()
	db, _ := newAccounts(t)
	for _, account := range [][3]string{
		{"local", "local", "correct horse"},
		{"Alice", "alice", "correct horse"},
		{"", "alice", "correct horse"},
		{"alice bob", "alice", "correct horse"},
		{strings.Repeat("a", 65), "alice", "correct horse"},
		{"alice", " ", "correct horse"},
		{"alice", "alice", "short"},
	} {
		if err := db.AddAccount(ctx, account[0], account[1], account[2]); !errors.Is(err, ErrInvalidAccount) {
			t.Errorf("account %q err=%v", account, err)
		}
	}
	if err := db.AddAccount(ctx, "alice", "alice", "correct horse"); err != nil {
		t.Fatal(err)
	}
	if err := db.AddAccount(ctx, "alice", "alice", "correct horse"); !errors.Is(err, ErrAccountExists) {
		t.Fatalf("duplicate err=%v", err)
	}
	if err := db.SetDisabled(ctx, "nobody", true); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("missing err=%v", err)
	}
}

func TestOpenRejectsMissingAndForeignFiles(t *testing.T) {
	ctx := context.Background()
	if _, err := Open(ctx, filepath.Join(t.TempDir(), "absent.sqlite")); err == nil {
		t.Fatal("opened an absent file")
	}
	foreign := filepath.Join(t.TempDir(), "foreign.sqlite")
	if err := os.WriteFile(foreign, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, foreign); !errors.Is(err, ErrIncompatible) {
		t.Fatalf("empty file err=%v", err)
	}
	// 作った file は、次の Create でも Open でも開ける。
	_, path := newAccounts(t)
	for _, reopen := range []func(context.Context, string) (*DB, error){Create, Open} {
		db, err := reopen(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// パスワードを確かめた後、セッションを記録する前にパスワードを変えたログインは、セッションを作らない。
func TestLoginRacingAPasswordChangeCreatesNoSession(t *testing.T) {
	ctx := context.Background()
	db, path := newAccounts(t)
	if err := db.AddAccount(ctx, "alice", "alice", "correct horse"); err != nil {
		t.Fatal(err)
	}
	operator, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer operator.Close()
	// Login は hash を確かめた直後に時刻を読む。その時点で運用者がパスワードを変える。
	db.now = func() time.Time {
		db.now = time.Now
		if err := operator.SetPassword(ctx, "alice", "another horse"); err != nil {
			t.Error(err)
		}
		return time.Now()
	}
	if _, err := db.Login(ctx, "alice", "correct horse"); !errors.Is(err, ErrLoginRejected) {
		t.Fatalf("err=%v", err)
	}
	var sessions int
	if err := db.db.QueryRow(`SELECT count(*) FROM session`).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("sessions=%d err=%v", sessions, err)
	}
}

// ログインは、期限の切れたセッションの行を消す。
func TestLoginRemovesExpiredSessions(t *testing.T) {
	ctx := context.Background()
	db, _ := newAccounts(t)
	clock := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	db.now = func() time.Time { return clock }
	if err := db.AddAccount(ctx, "alice", "alice", "correct horse"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Login(ctx, "alice", "correct horse"); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(SessionLifetime)
	if _, err := db.Login(ctx, "alice", "correct horse"); err != nil {
		t.Fatal(err)
	}
	var sessions int
	if err := db.db.QueryRow(`SELECT count(*) FROM session`).Scan(&sessions); err != nil || sessions != 1 {
		t.Fatalf("sessions=%d err=%v", sessions, err)
	}
}

// 作ったアカウントの file は、所有者だけが読める。
func TestCreatedFileIsReadableOnlyByTheOwner(t *testing.T) {
	_, path := newAccounts(t)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("mode=%v", mode)
	}
}

func TestPasswordHashUsesTheConfiguredIterationsAndVerifies(t *testing.T) {
	hash, err := hashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "pbkdf2-sha256$1000$") || strings.Contains(hash, "correct horse") {
		t.Fatalf("hash=%q", hash)
	}
	if !verifyPassword(hash, "correct horse") || verifyPassword(hash, "correct horsf") || verifyPassword("broken", "x") {
		t.Fatal("verification does not match the password")
	}
}
