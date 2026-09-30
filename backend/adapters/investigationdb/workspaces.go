package investigationdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// Workspace は、分析者が保存した画面の状態 1 件である。調査の原資料、導出の結果、分析者の判断を
// 持たない。
type Workspace struct {
	Id    string
	Owner string
	Name  string
	// State は画面の状態の JSON の文字列である。本 package は中身を解釈しない。
	State    string
	Revision int64
	// UpdatedAt は RFC 3339 の UTC の時刻である。
	UpdatedAt string
	UpdatedBy string
	// Shares は共有先の利用者を、ログイン名の順に並べる。
	Shares []WorkspaceShare
	// Changes は変更の記録を revision の順に並べる。
	Changes []WorkspaceChange
}

// WorkspaceChange は、ワークスペースの revision を 1 つ進めた変更 1 件の記録である。
type WorkspaceChange struct {
	WorkspaceId    string
	Revision       int64
	ClientChangeId string
	Actor          string
	// Fields は変更した state の最上位の欄名である。
	Fields []string
	// RecordedAt は RFC 3339 の UTC の時刻である。
	RecordedAt string
}

// WorkspaceShare は、ワークスペースを共有した利用者 1 人である。
type WorkspaceShare struct {
	Login string
	// Access は view または edit である。
	Access    string
	GrantedBy string
	// GrantedAt は RFC 3339 の UTC の時刻である。
	GrantedAt string
}

func (d *DB) readWorkspaces(ctx context.Context) ([]Workspace, error) {
	if exists, err := d.hasTable(ctx, "workspace"); err != nil || !exists {
		return nil, err
	}
	shares, err := d.readWorkspaceShares(ctx)
	if err != nil {
		return nil, err
	}
	changes, err := d.readWorkspaceChanges(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT id, owner, name, state, revision, updated_at, updated_by
		FROM workspace ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("reading the workspaces: %w", err)
	}
	defer rows.Close()
	var workspaces []Workspace
	for rows.Next() {
		var workspace Workspace
		if err := rows.Scan(&workspace.Id, &workspace.Owner, &workspace.Name, &workspace.State, &workspace.Revision,
			&workspace.UpdatedAt, &workspace.UpdatedBy); err != nil {
			return nil, fmt.Errorf("reading the workspaces: %w", err)
		}
		workspace.Shares, workspace.Changes = shares[workspace.Id], changes[workspace.Id]
		workspaces = append(workspaces, workspace)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the workspaces: %w", err)
	}
	return workspaces, nil
}

// readWorkspaceShares は共有をワークスペースの識別子ごとに返す。表を持たない file では空である。
func (d *DB) readWorkspaceShares(ctx context.Context) (map[string][]WorkspaceShare, error) {
	if exists, err := d.hasTable(ctx, "workspace_share"); err != nil || !exists {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT workspace_id, login, access, granted_by, granted_at
		FROM workspace_share ORDER BY workspace_id, login`)
	if err != nil {
		return nil, fmt.Errorf("reading the workspace shares: %w", err)
	}
	defer rows.Close()
	shares := map[string][]WorkspaceShare{}
	for rows.Next() {
		var id string
		var share WorkspaceShare
		if err := rows.Scan(&id, &share.Login, &share.Access, &share.GrantedBy, &share.GrantedAt); err != nil {
			return nil, fmt.Errorf("reading the workspace shares: %w", err)
		}
		shares[id] = append(shares[id], share)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the workspace shares: %w", err)
	}
	return shares, nil
}

// readWorkspaceChanges は変更の記録をワークスペースの識別子ごとに返す。表を持たない file では空である。
func (d *DB) readWorkspaceChanges(ctx context.Context) (map[string][]WorkspaceChange, error) {
	if exists, err := d.hasTable(ctx, "workspace_change"); err != nil || !exists {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT workspace_id, revision, client_change_id, actor, fields, recorded_at
		FROM workspace_change ORDER BY workspace_id, revision`)
	if err != nil {
		return nil, fmt.Errorf("reading the workspace changes: %w", err)
	}
	defer rows.Close()
	changes := map[string][]WorkspaceChange{}
	for rows.Next() {
		var change WorkspaceChange
		var fields string
		if err := rows.Scan(&change.WorkspaceId, &change.Revision, &change.ClientChangeId, &change.Actor, &fields,
			&change.RecordedAt); err != nil {
			return nil, fmt.Errorf("reading the workspace changes: %w", err)
		}
		if err := json.Unmarshal([]byte(fields), &change.Fields); err != nil {
			return nil, fmt.Errorf("reading the fields of the workspace change: %w", err)
		}
		changes[change.WorkspaceId] = append(changes[change.WorkspaceId], change)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the workspace changes: %w", err)
	}
	return changes, nil
}

// ReplaceWorkspaces は、workspaces を記録し (同じ識別子は共有も含めて置き換え)、changes を変更の記録へ
// 足し、removed の識別子のワークスペースと共有と変更の記録を消し、events を監査の記録へ足す。すべてを
// 1 つの transaction で行う。
func (d *DB) ReplaceWorkspaces(
	ctx context.Context, workspaces []Workspace, removed []string, changes []WorkspaceChange, events []AuditEvent,
) error {
	return d.transaction(ctx, func(tx *sql.Tx) error {
		if err := ensureAddedTables(ctx, tx); err != nil {
			return err
		}
		for _, workspace := range workspaces {
			if _, err := tx.ExecContext(ctx, `INSERT INTO workspace (id, owner, name, state, revision, updated_at, updated_by)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (id) DO UPDATE SET owner = excluded.owner, name = excluded.name, state = excluded.state,
					revision = excluded.revision, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				workspace.Id, workspace.Owner, workspace.Name, workspace.State, workspace.Revision, workspace.UpdatedAt,
				workspace.UpdatedBy); err != nil {
				return fmt.Errorf("recording the workspace: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM workspace_share WHERE workspace_id = ?`, workspace.Id); err != nil {
				return fmt.Errorf("recording the workspace shares: %w", err)
			}
			for _, share := range workspace.Shares {
				if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_share
					(workspace_id, login, access, granted_by, granted_at) VALUES (?, ?, ?, ?, ?)`,
					workspace.Id, share.Login, share.Access, share.GrantedBy, share.GrantedAt); err != nil {
					return fmt.Errorf("recording the workspace shares: %w", err)
				}
			}
		}
		for _, change := range changes {
			fields, err := json.Marshal(change.Fields)
			if err != nil {
				return fmt.Errorf("recording the workspace change: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_change
				(workspace_id, revision, client_change_id, actor, fields, recorded_at) VALUES (?, ?, ?, ?, ?, ?)`,
				change.WorkspaceId, change.Revision, change.ClientChangeId, change.Actor, string(fields),
				change.RecordedAt); err != nil {
				return fmt.Errorf("recording the workspace change: %w", err)
			}
		}
		for _, id := range removed {
			if _, err := tx.ExecContext(ctx, `DELETE FROM workspace_change WHERE workspace_id = ?`, id); err != nil {
				return fmt.Errorf("removing the workspace changes: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM workspace_share WHERE workspace_id = ?`, id); err != nil {
				return fmt.Errorf("removing the workspace shares: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM workspace WHERE id = ?`, id); err != nil {
				return fmt.Errorf("removing the workspace: %w", err)
			}
		}
		return insertAuditEvents(ctx, tx, events)
	})
}
