package investigationdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	_ "modernc.org/sqlite" // database/sql の driver "sqlite" を登録する。
)

// FileName は調査の directory の中の SQLite file の名前である。
const FileName = "investigation.sqlite"

// applicationId は SQLite の header に置く、Oraculum の調査の file であることの印である。
// 文字列 "ORCL" の byte 列を整数にした値である。
const applicationId = 0x4f52434c

// ErrIncompatible は、file が調査の file でないこと (header の印が applicationId と異なること) を
// 表す。
var ErrIncompatible = errors.New("investigation db: the file is not an investigation")

// ErrLocked は、別の process が同じ調査を開いていることを表す。
var ErrLocked = errors.New("investigation db: another process holds the investigation")

// ErrExists は、作成しようとした directory に既に調査の file があることを表す。
var ErrExists = errors.New("investigation db: the directory already holds an investigation")

// DB は開いた調査 1 件の SQLite file である。
//
// **接続を 1 本だけ持つ。** locking_mode=EXCLUSIVE の lock は接続に付くため、接続を
// 閉じると他の process が同じ file を開けるようになる。
type DB struct {
	db *sql.DB
}

// Meta は調査の file が作成時に記録した値である。
type Meta struct {
	// InvestigationId は調査の識別子である。作成時に乱数から作る。
	InvestigationId string
	// CreatedAt は調査を作成した時刻である。RFC 3339 の UTC。
	CreatedAt string
	// SourceRoot は、取り込みの指定の OriginPath を解決する基準の絶対 path である。
	SourceRoot string
}

// Contents は開いた調査の file が持つ値である。
type Contents struct {
	Meta Meta
	// Sources は取り込みの指定を、記録した順に並べる。
	Sources []Source
	// Assertions は所見を記録した順に並べる。
	Assertions []StoredAssertion
	// Assignments は分析者が与えた端末の割当を記録した順に並べる。
	Assignments []core.TerminalAssignment
	// Members は調査に参加する利用者の役割を、ログイン名の順に並べる。
	Members []Member
	// Workspaces は保存したワークスペースを、識別子の順に並べる。
	Workspaces []Workspace
	// AssistPermissions は送信の許可の改訂を記録した順に並べる。
	AssistPermissions []core.AssistPermissionRevision
	// AssistConversations は AI 支援の会話と、その受け渡しの記録である。
	AssistConversations AssistConversations
	// SkippedFiles は、収集の directory にあり取り込まなかった file を、記録した順に並べる。
	SkippedFiles []core.SkippedFile
	// AssistProposals は AI 提案を記録した順に並べる。
	AssistProposals []StoredAssistProposal
}

// Exists は dir に調査の file があるかを返す。
func Exists(dir string) (bool, error) {
	info, err := os.Stat(filepath.Join(dir, FileName))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("checking the investigation file in %q: %w", dir, err)
	case !info.Mode().IsRegular():
		return false, fmt.Errorf("checking the investigation file in %q: %s is not a regular file", dir, FileName)
	}
	return true, nil
}

// Create は dir に調査の file を作り、sourceRoot と取り込みの指定 sources を記録して開く。
//
// **作成と最初の取り込みの指定の記録を 1 つの transaction で行う。** 指定を持たない調査の
// file を残さない。失敗したときは、本関数が作った file を消す。
func Create(ctx context.Context, dir, sourceRoot string, sources []Source) (*DB, error) {
	if !filepath.IsAbs(sourceRoot) {
		return nil, fmt.Errorf("creating the investigation: the source root %q is not absolute", sourceRoot)
	}
	if len(sources) == 0 {
		return nil, errors.New("creating the investigation: at least one source is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating the investigation directory %q: %w", dir, err)
	}
	path := filepath.Join(dir, FileName)
	// O_EXCL で作ることで、同じ directory を同時に作成する 2 つの起動の片方を退ける。
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- 運用者が起動引数で指定した調査の directory に作る。
	if errors.Is(err, fs.ErrExist) {
		return nil, fmt.Errorf("creating the investigation in %q: %w", dir, ErrExists)
	}
	if err != nil {
		return nil, fmt.Errorf("creating the investigation file %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("creating the investigation file %q: %w", path, err)
	}
	db, err := create(ctx, path, sourceRoot, sources)
	if err != nil {
		return nil, errors.Join(err, removeCreated(path))
	}
	return db, nil
}

func create(ctx context.Context, path, sourceRoot string, sources []Source) (*DB, error) {
	db, _, err := open(path)
	if err != nil {
		return nil, err
	}
	if err := db.hold(ctx, path); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	id, err := newInvestigationId()
	if err != nil {
		return nil, errors.Join(err, db.Close())
	}
	meta := Meta{InvestigationId: id, CreatedAt: time.Now().UTC().Format(time.RFC3339), SourceRoot: sourceRoot}
	err = db.transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA application_id = %d", applicationId)); err != nil {
			return fmt.Errorf("setting the application id: %w", err)
		}
		for _, statement := range schema {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("creating the tables: %w", err)
			}
		}
		for key, value := range map[string]string{
			metaInvestigationId: meta.InvestigationId, metaCreatedAt: meta.CreatedAt,
			metaSourceRoot: meta.SourceRoot,
		} {
			if _, err := tx.ExecContext(ctx, `INSERT INTO meta (key, value) VALUES (?, ?)`, key, value); err != nil {
				return fmt.Errorf("recording %s: %w", key, err)
			}
		}
		return insertSources(ctx, tx, sources)
	})
	if err != nil {
		return nil, errors.Join(fmt.Errorf("creating the investigation file %q: %w", path, err), db.Close())
	}
	return db, nil
}

// removeCreated は Create が作った file と、SQLite が横に作った journal を消す。
func removeCreated(path string) error {
	var problems []error
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
			problems = append(problems, fmt.Errorf("removing the partly created %q: %w", path+suffix, err))
		}
	}
	return errors.Join(problems...)
}

// Open は dir の調査の file を開き、記録した値を読む。
//
// 調査の file でない file は ErrIncompatible、別の process が開いている file は
// ErrLocked を包んで返す。どちらの場合も file を変更しない。
func Open(ctx context.Context, dir string) (*DB, Contents, error) {
	path := filepath.Join(dir, FileName)
	db, id, err := open(path)
	if err != nil {
		return nil, Contents{}, err
	}
	contents, err := db.checkAndRead(ctx, path, id)
	if err != nil {
		return nil, Contents{}, errors.Join(fmt.Errorf("opening the investigation %q: %w", dir, err), db.Close())
	}
	return db, contents, nil
}

// checkAndRead は file の印と meta を読むだけで確かめ、確かめた後に排他の lock を取って
// 記録した値をすべて読む。
func (d *DB) checkAndRead(ctx context.Context, path string, id int64) (Contents, error) {
	if id != applicationId {
		// 作成は WAL へ切り替えた後、表を作る transaction を commit する前に止まり得る。
		// 空の file と表を 1 つも持たない file を、作成の途中で止まった file と読む。
		var objects int64
		if err := d.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema`).Scan(&objects); err != nil {
			return Contents{}, fmt.Errorf("reading the schema: %w", err)
		}
		if id == 0 && objects == 0 {
			return Contents{}, fmt.Errorf("the file is empty, a creation stopped before it finished; "+
				"remove the file to create the investigation again: %w", ErrIncompatible)
		}
		return Contents{}, fmt.Errorf("the file is not an investigation (application id %d): %w", id, ErrIncompatible)
	}
	meta, err := d.readMeta(ctx)
	if err != nil {
		return Contents{}, err
	}
	if err := d.hold(ctx, path); err != nil {
		return Contents{}, err
	}
	return d.read(ctx, meta)
}

// open は path の file へ接続を 1 本張り、file の印 (application_id) を読む。file を作らず、
// 書き換えない。別の process が排他の lock を持っていれば ErrLocked を返す。
func open(path string) (*DB, int64, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, 0, fmt.Errorf("resolving the investigation file %q: %w", path, err)
	}
	// URI の path は百分率符号化する。空白・?・# を含む directory 名を query と取り違えない。
	// journal_mode は hold で、file を確かめた後に設定する。
	dsn := "file:" + (&url.URL{Path: absolute}).EscapedPath() + "?mode=rw" +
		"&_pragma=locking_mode(EXCLUSIVE)&_pragma=synchronous(FULL)" +
		"&_pragma=foreign_keys(1)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, 0, fmt.Errorf("opening the investigation file %q: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)
	var id int64
	if err := db.QueryRow(`PRAGMA application_id`).Scan(&id); err != nil {
		return nil, 0, errors.Join(lockError(path, err), db.Close())
	}
	return &DB{db: db}, id, nil
}

// hold は file を WAL にし、空の書き込みの transaction で排他の lock を取る。
//
// **読み取りだけの接続は共有の lock しか持たない。** 開き直した直後の起動は何も書かないため、
// 書き込みの lock をここで取らなければ、2 つ目の process も同じ調査を開けてしまう。
func (d *DB) hold(ctx context.Context, path string) error {
	var mode string
	if err := d.db.QueryRowContext(ctx, `PRAGMA journal_mode = WAL`).Scan(&mode); err != nil {
		return lockError(path, err)
	}
	if mode != "wal" {
		return fmt.Errorf("opening the investigation file %q: the journal mode is %q, want wal", path, mode)
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return lockError(path, err)
	}
	if err := tx.Commit(); err != nil {
		return lockError(path, err)
	}
	return nil
}

// lockError は lock を取る操作の失敗を、lock の競合とそれ以外に分ける。
func lockError(path string, err error) error {
	if isBusy(err) {
		return fmt.Errorf("opening the investigation file %q: %w", path, ErrLocked)
	}
	return fmt.Errorf("opening the investigation file %q: %w", path, err)
}

// Close は接続を閉じ、排他の lock を放す。
func (d *DB) Close() error {
	if err := d.db.Close(); err != nil {
		return fmt.Errorf("closing the investigation file: %w", err)
	}
	return nil
}

// read は meta の後に記録した値をすべて読む。
func (d *DB) read(ctx context.Context, meta Meta) (Contents, error) {
	sources, err := d.readSources(ctx)
	if err != nil {
		return Contents{}, err
	}
	assertions, err := d.readAssertions(ctx)
	if err != nil {
		return Contents{}, err
	}
	assignments, err := d.readAssignments(ctx)
	if err != nil {
		return Contents{}, err
	}
	members, err := d.readMembers(ctx)
	if err != nil {
		return Contents{}, err
	}
	workspaces, err := d.readWorkspaces(ctx)
	if err != nil {
		return Contents{}, err
	}
	permissions, err := d.readAssistPermissions(ctx)
	if err != nil {
		return Contents{}, err
	}
	conversations, err := d.readAssistConversations(ctx)
	if err != nil {
		return Contents{}, err
	}
	skipped, err := d.readSkippedFiles(ctx)
	if err != nil {
		return Contents{}, err
	}
	proposals, err := d.readAssistProposals(ctx)
	if err != nil {
		return Contents{}, err
	}
	return Contents{Meta: meta, Sources: sources, Assertions: assertions, Assignments: assignments,
		Members: members, Workspaces: workspaces,
		AssistPermissions: permissions, AssistConversations: conversations, SkippedFiles: skipped,
		AssistProposals: proposals}, nil
}

// meta の表の鍵。
const (
	metaInvestigationId = "investigation_id"
	metaCreatedAt       = "created_at"
	metaSourceRoot      = "source_root"
)

func (d *DB) readMeta(ctx context.Context) (Meta, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT key, value FROM meta`)
	if err != nil {
		return Meta{}, fmt.Errorf("reading the meta table: %w", err)
	}
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return Meta{}, fmt.Errorf("reading the meta table: %w", err)
		}
		values[key] = value
	}
	if err := rows.Err(); err != nil {
		return Meta{}, fmt.Errorf("reading the meta table: %w", err)
	}
	meta := Meta{
		InvestigationId: values[metaInvestigationId], CreatedAt: values[metaCreatedAt],
		SourceRoot: values[metaSourceRoot],
	}
	if meta.InvestigationId == "" || meta.CreatedAt == "" || !filepath.IsAbs(meta.SourceRoot) {
		return Meta{}, errors.New("the meta table lacks the investigation id, the creation time or an absolute source root")
	}
	return meta, nil
}

// transaction は body を 1 つの transaction で行う。body が失敗したら巻き戻す。
func (d *DB) transaction(ctx context.Context, body func(*sql.Tx) error) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning a transaction: %w", err)
	}
	if err := body(tx); err != nil {
		return errors.Join(err, rollback(tx))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing the transaction: %w", err)
	}
	return nil
}

func rollback(tx *sql.Tx) error {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return fmt.Errorf("rolling back the transaction: %w", err)
	}
	return nil
}

func newInvestigationId() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("creating the investigation id: %w", err)
	}
	return "inv:" + hex.EncodeToString(bytes[:]), nil
}
