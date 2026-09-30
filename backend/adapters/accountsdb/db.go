// Package accountsdb は、server に接続する利用者のアカウントとセッションを 1 つの SQLite file に置く。
//
// **調査の file と分ける。** credential を調査と同じ方法で配布しない。file は排他の lock を取らず、
// server が開いている間も運用者の command が同じ file を書ける。無効化とセッションの失効は、
// server の次の要求の検査に現れる。
package accountsdb

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // database/sql の driver "sqlite" を登録する。
)

// applicationId は SQLite の header に置く、Oraculum のアカウントの file であることの印である。
// 文字列 "ORCA" の byte 列を整数にした値である。
const applicationId = 0x4f524341

// ReservedLogin は、アカウントのログイン名に使えない予約した名前である。
const ReservedLogin = "local"

// SessionLifetime は、ログインしてからセッションが切れるまでの時間である。
const SessionLifetime = 12 * time.Hour

// MinPasswordLength はパスワードの最小の文字数である。
const MinPasswordLength = 8

// hashIterations は PBKDF2-HMAC-SHA256 の反復回数である。OWASP の 2023 年の推奨値である。
var hashIterations = 600_000

var (
	// ErrIncompatible は、file がアカウントの file でないことを表す。
	ErrIncompatible = errors.New("accounts db: the file is not an accounts file")
	// ErrAccountExists は、追加しようとしたログイン名のアカウントが既にあることを表す。
	ErrAccountExists = errors.New("accounts db: the login already exists")
	// ErrAccountNotFound は、ログイン名のアカウントが無いことを表す。
	ErrAccountNotFound = errors.New("accounts db: no account has the login")
	// ErrInvalidAccount は、ログイン名・表示名・パスワードが値の条件に合わないことを表す。
	ErrInvalidAccount = errors.New("accounts db: the account value is invalid")
	// ErrLoginRejected は、ログイン名とパスワードの組を受け付けないことを表す。アカウントが無い、
	// パスワードが違う、アカウントを無効にした、のどれであるかを区別しない。
	ErrLoginRejected = errors.New("accounts db: the login and the password are rejected")
	// ErrSessionInvalid は、token のセッションが無い、切れた、失効した、または利用者を無効にした
	// ことを表す。
	ErrSessionInvalid = errors.New("accounts db: the session is not valid")
)

// loginPattern はログイン名の文字列の条件である。
var loginPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

var schema = []string{
	`CREATE TABLE account (
		login TEXT PRIMARY KEY,
		display_name TEXT NOT NULL,
		password_hash TEXT NOT NULL,
		created_at TEXT NOT NULL,
		disabled_at TEXT
	) STRICT`,
	`CREATE TABLE session (
		token_sha256 TEXT PRIMARY KEY,
		login TEXT NOT NULL REFERENCES account (login),
		created_at TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		revoked_at TEXT
	) STRICT`,
}

// Account はアカウント 1 件の、利用者を識別する値である。
type Account struct {
	// Login は利用者の安定した識別子である。変更しない。
	Login string
	// DisplayName は画面に出す名前である。
	DisplayName string
}

// Session はログインで作ったセッションである。
type Session struct {
	// Token は browser に渡す secret である。file は sha256 だけを持つ。
	Token     string
	Account   Account
	ExpiresAt time.Time
}

// DB は開いたアカウントの file である。
type DB struct {
	db  *sql.DB
	now func() time.Time
}

// Create は path にアカウントの file を作って開く。path に file があれば、アカウントの file で
// あることを確かめて開く。
func Create(ctx context.Context, path string) (*DB, error) {
	return open(ctx, path, true)
}

// Open は path の既存のアカウントの file を開く。
func Open(ctx context.Context, path string) (*DB, error) {
	return open(ctx, path, false)
}

func open(ctx context.Context, path string, create bool) (*DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving the accounts file %q: %w", path, err)
	}
	mode := "rw"
	if create {
		mode = "rwc"
		// パスワードの hash を同じ host の他の利用者に読ませない。SQLite は WAL と shm の file を
		// 本体と同じ権限で作る。既にある file の権限は変えない。
		file, err := os.OpenFile(absolute, os.O_RDWR|os.O_CREATE, 0o600) // #nosec G304 -- 運用者が指定したアカウントの file を作る。
		if err != nil {
			return nil, fmt.Errorf("creating the accounts file %q: %w", path, err)
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("creating the accounts file %q: %w", path, err)
		}
	}
	// URI の path は百分率符号化する。空白・?・# を含む directory 名を query と取り違えない。
	dsn := "file:" + (&url.URL{Path: absolute}).EscapedPath() + "?mode=" + mode +
		"&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)" +
		"&_pragma=foreign_keys(1)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening the accounts file %q: %w", path, err)
	}
	accounts := &DB{db: db, now: time.Now}
	if err := accounts.prepare(ctx, create); err != nil {
		return nil, errors.Join(fmt.Errorf("opening the accounts file %q: %w", path, err), db.Close())
	}
	return accounts, nil
}

// prepare は file の印を確かめる。空の file は、create のときだけ表を作る。
func (d *DB) prepare(ctx context.Context, create bool) error {
	return d.transaction(ctx, func(tx *sql.Tx) error {
		var id, objects int64
		if err := tx.QueryRowContext(ctx, `PRAGMA application_id`).Scan(&id); err != nil {
			return fmt.Errorf("reading the application id: %w", err)
		}
		if id == applicationId {
			return nil
		}
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema`).Scan(&objects); err != nil {
			return fmt.Errorf("reading the schema: %w", err)
		}
		if id != 0 || objects != 0 || !create {
			return ErrIncompatible
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA application_id = %d", applicationId)); err != nil {
			return fmt.Errorf("setting the application id: %w", err)
		}
		for _, statement := range schema {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("creating the tables: %w", err)
			}
		}
		return nil
	})
}

// Close は file を閉じる。
func (d *DB) Close() error {
	if err := d.db.Close(); err != nil {
		return fmt.Errorf("closing the accounts file: %w", err)
	}
	return nil
}

// AddAccount はアカウントを 1 件足す。
func (d *DB) AddAccount(ctx context.Context, login, displayName, password string) error {
	if err := checkLogin(login); err != nil {
		return err
	}
	if strings.TrimSpace(displayName) == "" || len(displayName) > 256 {
		return fmt.Errorf("the display name must have 1 to 256 bytes: %w", ErrInvalidAccount)
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	return d.transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `INSERT INTO account (login, display_name, password_hash, created_at)
			VALUES (?, ?, ?, ?) ON CONFLICT (login) DO NOTHING`, login, displayName, hash, d.timestamp(d.now()))
		if err != nil {
			return fmt.Errorf("adding the account: %w", err)
		}
		if affected, err := result.RowsAffected(); err != nil || affected == 0 {
			return errors.Join(ErrAccountExists, err)
		}
		return nil
	})
}

// SetPassword はパスワードを変え、その利用者のセッションをすべて失効させる。
func (d *DB) SetPassword(ctx context.Context, login, password string) error {
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	return d.transaction(ctx, func(tx *sql.Tx) error {
		if err := updateAccount(ctx, tx, `UPDATE account SET password_hash = ? WHERE login = ?`, hash, login); err != nil {
			return err
		}
		return d.revokeSessions(ctx, tx, login)
	})
}

// SetDisabled は利用者を無効または有効にする。無効にするときは、その利用者のセッションを
// すべて失効させる。
func (d *DB) SetDisabled(ctx context.Context, login string, disabled bool) error {
	return d.transaction(ctx, func(tx *sql.Tx) error {
		if !disabled {
			return updateAccount(ctx, tx, `UPDATE account SET disabled_at = NULL WHERE login = ?`, login)
		}
		err := updateAccount(ctx, tx, `UPDATE account SET disabled_at = coalesce(disabled_at, ?) WHERE login = ?`,
			d.timestamp(d.now()), login)
		if err != nil {
			return err
		}
		return d.revokeSessions(ctx, tx, login)
	})
}

// Login はログイン名とパスワードを確かめ、新しいセッションを作る。
//
// **アカウントの有無で所要の時間を変えない。** アカウントが無いときも同じ回数の hash を求める。
func (d *DB) Login(ctx context.Context, login, password string) (Session, error) {
	var displayName, stored string
	var disabled sql.NullString
	err := d.db.QueryRowContext(ctx, `SELECT display_name, password_hash, disabled_at FROM account WHERE login = ?`,
		login).Scan(&displayName, &stored, &disabled)
	if errors.Is(err, sql.ErrNoRows) {
		_ = verifyPassword(dummyHash(), password)
		return Session{}, ErrLoginRejected
	}
	if err != nil {
		return Session{}, fmt.Errorf("reading the account: %w", err)
	}
	if !verifyPassword(stored, password) || disabled.Valid {
		return Session{}, ErrLoginRejected
	}
	token, err := newToken()
	if err != nil {
		return Session{}, err
	}
	created := d.now()
	expires := created.Add(SessionLifetime)
	// **確かめた hash のままのアカウントにだけセッションを作る。** hash を求める間にパスワードを
	// 変えた、または無効にしたアカウントに、古いパスワードのセッションを残さない。
	var inserted int64
	err = d.transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM session WHERE expires_at <= ?`, d.timestamp(created)); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO session (token_sha256, login, created_at, expires_at)
			SELECT ?, ?, ?, ? WHERE EXISTS (
				SELECT 1 FROM account WHERE login = ? AND password_hash = ? AND disabled_at IS NULL)`,
			tokenDigest(token), login, d.timestamp(created), d.timestamp(expires), login, stored)
		if err != nil {
			return err
		}
		inserted, err = result.RowsAffected()
		return err
	})
	if err != nil {
		return Session{}, fmt.Errorf("recording the session: %w", err)
	}
	if inserted == 0 {
		return Session{}, ErrLoginRejected
	}
	return Session{Token: token, Account: Account{Login: login, DisplayName: displayName}, ExpiresAt: expires}, nil
}

// Account はログイン名のアカウントを返す。無効にしたアカウントも返す。
func (d *DB) Account(ctx context.Context, login string) (Account, error) {
	account := Account{Login: login}
	err := d.db.QueryRowContext(ctx, `SELECT display_name FROM account WHERE login = ?`, login).
		Scan(&account.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrAccountNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("reading the account: %w", err)
	}
	return account, nil
}

// SessionAccount は token のセッションの利用者を返す。セッションが切れた、失効した、または利用者を
// 無効にしたときは ErrSessionInvalid を返す。
func (d *DB) SessionAccount(ctx context.Context, token string) (Account, error) {
	var account Account
	err := d.db.QueryRowContext(ctx, `SELECT account.login, account.display_name
		FROM session JOIN account ON account.login = session.login
		WHERE session.token_sha256 = ? AND session.revoked_at IS NULL AND session.expires_at > ?
			AND account.disabled_at IS NULL`, tokenDigest(token), d.timestamp(d.now())).
		Scan(&account.Login, &account.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrSessionInvalid
	}
	if err != nil {
		return Account{}, fmt.Errorf("reading the session: %w", err)
	}
	return account, nil
}

// Logout は token のセッションを失効させる。無いセッションは何もしない。
func (d *DB) Logout(ctx context.Context, token string) error {
	return d.transaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE session SET revoked_at = coalesce(revoked_at, ?) WHERE token_sha256 = ?`,
			d.timestamp(d.now()), tokenDigest(token))
		return err
	})
}

func (d *DB) revokeSessions(ctx context.Context, tx *sql.Tx, login string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE session SET revoked_at = ? WHERE login = ? AND revoked_at IS NULL`,
		d.timestamp(d.now()), login); err != nil {
		return fmt.Errorf("revoking the sessions: %w", err)
	}
	return nil
}

func updateAccount(ctx context.Context, tx *sql.Tx, statement string, args ...any) error {
	result, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		return fmt.Errorf("updating the account: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		return errors.Join(ErrAccountNotFound, err)
	}
	return nil
}

// timestamp は時刻を、文字列の順序が時刻の順序と一致する UTC の文字列にする。
func (d *DB) timestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

// transaction は body を 1 つの transaction で行う。body が失敗したら巻き戻す。
func (d *DB) transaction(ctx context.Context, body func(*sql.Tx) error) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning a transaction: %w", err)
	}
	if err := body(tx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return errors.Join(err, rollbackErr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing the transaction: %w", err)
	}
	return nil
}

func checkLogin(login string) error {
	if login == ReservedLogin || !loginPattern.MatchString(login) {
		return fmt.Errorf("the login must be 1 to 64 of a-z, 0-9, '.', '_', '-', start with a letter or a digit, "+
			"and differ from %q: %w", ReservedLogin, ErrInvalidAccount)
	}
	return nil
}

// dummyHash は、無いアカウントへのログインで hash を求めるための値である。反復回数は、アカウントの
// hash と同じ hashIterations である。
func dummyHash() string {
	return "pbkdf2-sha256$" + strconv.Itoa(hashIterations) + "$" +
		base64.RawStdEncoding.EncodeToString(make([]byte, 16)) + "$" +
		base64.RawStdEncoding.EncodeToString(make([]byte, sha256.Size))
}

// hashPassword はパスワードを `pbkdf2-sha256$<反復回数>$<salt>$<hash>` の文字列にする。
func hashPassword(password string) (string, error) {
	if len([]rune(password)) < MinPasswordLength {
		return "", fmt.Errorf("the password must have at least %d characters: %w", MinPasswordLength, ErrInvalidAccount)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("creating the salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, hashIterations, sha256.Size)
	if err != nil {
		return "", fmt.Errorf("hashing the password: %w", err)
	}
	return "pbkdf2-sha256$" + strconv.Itoa(hashIterations) + "$" + base64.RawStdEncoding.EncodeToString(salt) +
		"$" + base64.RawStdEncoding.EncodeToString(key), nil
}

func verifyPassword(stored, password string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return false
	}
	salt, saltErr := base64.RawStdEncoding.DecodeString(parts[2])
	want, wantErr := base64.RawStdEncoding.DecodeString(parts[3])
	if saltErr != nil || wantErr != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

func newToken() (string, error) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return "", fmt.Errorf("creating the session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(token), nil
}

func tokenDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}
