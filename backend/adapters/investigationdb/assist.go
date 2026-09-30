package investigationdb

import (
	"context"
	"database/sql"
	"fmt"
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// assistSchema は AI 支援の記録の表である。
//
// **既存の表と列を変えない。** 表は最初に AI 支援の記録を書く transaction が作る。AI 支援の
// 表を持たない調査の file も開き、記録が無いものとして読む。
var assistSchema = []string{
	`CREATE TABLE IF NOT EXISTS assist_permission_revision (
		position INTEGER PRIMARY KEY,
		provider TEXT NOT NULL,
		revision_number INTEGER NOT NULL CHECK (revision_number >= 1),
		action TEXT NOT NULL CHECK (action IN ('grant', 'revoke')),
		analyst TEXT NOT NULL,
		recorded_at TEXT NOT NULL,
		UNIQUE (provider, revision_number)
	)`,
}

// assistTransaction は、AI 支援の表を作ってから body を同じ transaction で行う。
func (d *DB) assistTransaction(ctx context.Context, body func(*sql.Tx) error) error {
	return d.transaction(ctx, func(tx *sql.Tx) error {
		for _, statement := range append(slices.Clone(assistSchema), assistConversationSchema...) {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("creating the assist tables: %w", err)
			}
		}
		return body(tx)
	})
}

// InsertAssistPermission は送信の許可の改訂 1 つを、1 つの transaction で書く。
func (d *DB) InsertAssistPermission(ctx context.Context, revision core.AssistPermissionRevision) error {
	err := d.assistTransaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO assist_permission_revision (provider, revision_number,
			action, analyst, recorded_at) VALUES (?, ?, ?, ?, ?)`, string(revision.Provider),
			revision.RevisionNumber, string(revision.Action), revision.Analyst,
			string(revision.RecordedAt)); err != nil {
			return fmt.Errorf("recording the assist permission: %w", err)
		}
		return nil
	})
	if err != nil {
		// 分析者が書いた文字列を error に載せない。呼び出し元が log へ出す。
		return fmt.Errorf("recording the assist permission: %w", err)
	}
	return nil
}

// readAssistPermissions は送信の許可の改訂を記録した順に読み、core の Validate を通して返す。
// 表を持たない file では空を返す。
func (d *DB) readAssistPermissions(ctx context.Context) ([]core.AssistPermissionRevision, error) {
	exists, err := d.hasTable(ctx, "assist_permission_revision")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT position, provider, revision_number, action, analyst, recorded_at
		FROM assist_permission_revision ORDER BY position`)
	if err != nil {
		return nil, fmt.Errorf("reading the assist permissions: %w", err)
	}
	defer rows.Close()
	var revisions []core.AssistPermissionRevision
	for rows.Next() {
		var position int64
		var provider, action, recordedAt string
		var revision core.AssistPermissionRevision
		if err := rows.Scan(&position, &provider, &revision.RevisionNumber, &action, &revision.Analyst,
			&recordedAt); err != nil {
			return nil, fmt.Errorf("reading the assist permissions: %w", err)
		}
		revision.Provider = core.AssistProvider(provider)
		revision.Action = core.AssistPermissionAction(action)
		revision.RecordedAt = core.AssertionTime(recordedAt)
		if err := revision.Validate(); err != nil {
			return nil, fmt.Errorf("reading the assist permission %d: %w", position, err)
		}
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the assist permissions: %w", err)
	}
	return revisions, nil
}
