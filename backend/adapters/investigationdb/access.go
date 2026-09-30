package investigationdb

import (
	"context"
	"database/sql"
	"fmt"
)

// addedSchema は、調査の file を作った後に足した表である。**表を持たない file も開ける。** 読むときは
// 表が無ければ空とし、書くときは同じ transaction で表を作る。既存の表と列を変えない。
var addedSchema = []string{
	`CREATE TABLE IF NOT EXISTS member (
		login TEXT PRIMARY KEY,
		role TEXT NOT NULL,
		granted_by TEXT NOT NULL,
		granted_at TEXT NOT NULL
	) WITHOUT ROWID`,
	`CREATE TABLE IF NOT EXISTS audit_event (
		position INTEGER PRIMARY KEY,
		recorded_at TEXT NOT NULL,
		actor TEXT NOT NULL,
		action TEXT NOT NULL,
		target TEXT NOT NULL,
		detail TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS workspace (
		id TEXT PRIMARY KEY,
		owner TEXT NOT NULL,
		name TEXT NOT NULL,
		state TEXT NOT NULL,
		revision INTEGER NOT NULL,
		updated_at TEXT NOT NULL,
		updated_by TEXT NOT NULL
	) WITHOUT ROWID`,
	`CREATE TABLE IF NOT EXISTS workspace_share (
		workspace_id TEXT NOT NULL,
		login TEXT NOT NULL,
		access TEXT NOT NULL,
		granted_by TEXT NOT NULL,
		granted_at TEXT NOT NULL,
		PRIMARY KEY (workspace_id, login)
	) WITHOUT ROWID`,
	`CREATE TABLE IF NOT EXISTS workspace_change (
		workspace_id TEXT NOT NULL,
		revision INTEGER NOT NULL,
		client_change_id TEXT NOT NULL,
		actor TEXT NOT NULL,
		fields TEXT NOT NULL,
		recorded_at TEXT NOT NULL,
		PRIMARY KEY (workspace_id, revision),
		UNIQUE (workspace_id, client_change_id)
	) WITHOUT ROWID`,
	`CREATE TABLE IF NOT EXISTS skipped_file (
		position INTEGER PRIMARY KEY,
		origin_path TEXT NOT NULL UNIQUE,
		reason TEXT NOT NULL,
		detected_kind TEXT NOT NULL
	)`,
}

// Member は調査に参加する利用者 1 人の役割である。
type Member struct {
	Login string
	Role  string
	// GrantedBy は役割を与えた利用者のログイン名、または運用者の起動引数である。
	GrantedBy string
	// GrantedAt は RFC 3339 の UTC の時刻である。
	GrantedAt string
}

// AuditEvent は、権限と共有の変更 1 件の記録である。
type AuditEvent struct {
	// RecordedAt は RFC 3339 の UTC の時刻である。
	RecordedAt string
	Actor      string
	Action     string
	Target     string
	// Detail は変更の後の値である。credential と原資料の本文を持たない。
	Detail string
}

// ensureAddedTables は addedSchema の表を tx の中で作る。
func ensureAddedTables(ctx context.Context, tx *sql.Tx) error {
	for _, statement := range addedSchema {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("creating the added tables: %w", err)
		}
	}
	return nil
}

// hasTable は file が name の表を持つかを返す。
func (d *DB) hasTable(ctx context.Context, name string) (bool, error) {
	var count int
	if err := d.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = ?`,
		name).Scan(&count); err != nil {
		return false, fmt.Errorf("reading the schema: %w", err)
	}
	return count == 1, nil
}

func (d *DB) readMembers(ctx context.Context) ([]Member, error) {
	if exists, err := d.hasTable(ctx, "member"); err != nil || !exists {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT login, role, granted_by, granted_at FROM member ORDER BY login`)
	if err != nil {
		return nil, fmt.Errorf("reading the members: %w", err)
	}
	defer rows.Close()
	var members []Member
	for rows.Next() {
		var member Member
		if err := rows.Scan(&member.Login, &member.Role, &member.GrantedBy, &member.GrantedAt); err != nil {
			return nil, fmt.Errorf("reading the members: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the members: %w", err)
	}
	return members, nil
}

// ReplaceMembers は、members の役割を記録し (既にある利用者は置き換え)、removed の利用者の役割を消し、
// events を監査の記録へ足す。すべてを 1 つの transaction で行う。
func (d *DB) ReplaceMembers(ctx context.Context, members []Member, removed []string, events []AuditEvent) error {
	return d.transaction(ctx, func(tx *sql.Tx) error {
		if err := ensureAddedTables(ctx, tx); err != nil {
			return err
		}
		for _, member := range members {
			if _, err := tx.ExecContext(ctx, `INSERT INTO member (login, role, granted_by, granted_at) VALUES (?, ?, ?, ?)
				ON CONFLICT (login) DO UPDATE SET role = excluded.role, granted_by = excluded.granted_by,
					granted_at = excluded.granted_at`,
				member.Login, member.Role, member.GrantedBy, member.GrantedAt); err != nil {
				return fmt.Errorf("recording the member: %w", err)
			}
		}
		for _, login := range removed {
			if _, err := tx.ExecContext(ctx, `DELETE FROM member WHERE login = ?`, login); err != nil {
				return fmt.Errorf("removing the member: %w", err)
			}
		}
		return insertAuditEvents(ctx, tx, events)
	})
}

// insertAuditEvents は events を tx の中で監査の記録へ足す。
func insertAuditEvents(ctx context.Context, tx *sql.Tx, events []AuditEvent) error {
	for _, event := range events {
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_event (recorded_at, actor, action, target, detail)
			VALUES (?, ?, ?, ?, ?)`, event.RecordedAt, event.Actor, event.Action, event.Target, event.Detail); err != nil {
			return fmt.Errorf("recording the audit event: %w", err)
		}
	}
	return nil
}

// AuditEvents は監査の記録を、記録した順に返す。
func (d *DB) AuditEvents(ctx context.Context) ([]AuditEvent, error) {
	if exists, err := d.hasTable(ctx, "audit_event"); err != nil || !exists {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT recorded_at, actor, action, target, detail FROM audit_event
		ORDER BY position`)
	if err != nil {
		return nil, fmt.Errorf("reading the audit events: %w", err)
	}
	defer rows.Close()
	var events []AuditEvent
	for rows.Next() {
		var event AuditEvent
		if err := rows.Scan(&event.RecordedAt, &event.Actor, &event.Action, &event.Target, &event.Detail); err != nil {
			return nil, fmt.Errorf("reading the audit events: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the audit events: %w", err)
	}
	return events, nil
}
