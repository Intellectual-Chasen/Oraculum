package investigationdb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// Source は収集元 1 件の取り込みの指定である。
type Source struct {
	// CaseId は収集元に付けた案件である。nil は案件を区別しない取り込みを表す。
	CaseId *string
	// FormatKey は入力形式である。
	FormatKey core.FormatKey
	// OriginPath は収集元の取得元である。取り込みの基準 directory からの相対 path。
	OriginPath string
	// FormatSpec は起動が渡した欄の並びである。nil は入力形式そのものが並びを定めることを表す。
	FormatSpec *string
	// ContentSha256 は記録した時点の収集元の内容の識別である。
	ContentSha256 string
	// ImportOrdinal は同じ取得元と内容の組に対する取り込みの通番である。sourceId の材料になる。
	ImportOrdinal int64
	// Terminal は利用者が取り込みの起動で指定した、収集元を記録した端末である。nil は
	// 指定していないことを表す。
	Terminal *Terminal
	// CollectionPath は収集元を取り出した収集の directory である。空の文字列は、収集元の file を
	// 1 件ずつ指定したことを表す。
	CollectionPath string
}

// Terminal は収集元 1 件に指定した端末と、時刻を読む UTC からのずれである。省いた項目は空の
// 文字列か nil で持つ。
type Terminal struct {
	// Id は端末の外部識別子である。
	Id string
	// Hostname は端末の表示名である。
	Hostname string
	// Ip は端末が持つ IP アドレスである。
	Ip string
	// TimeOffset は収集元の時刻を読む UTC からのずれである。nil は指定していないことを表す。
	TimeOffset *core.UtcOffset
}

// terminalColumns は端末の 4 項目を列の値にする。端末を指定していない収集元と省いた項目は
// NULL にする。
func terminalColumns(terminal *Terminal) (id, hostname, ip, offset sql.NullString) {
	if terminal == nil {
		return
	}
	present := func(value string) sql.NullString {
		return sql.NullString{String: value, Valid: value != ""}
	}
	if terminal.TimeOffset != nil {
		offset = present(string(*terminal.TimeOffset))
	}
	return present(terminal.Id), present(terminal.Hostname), present(terminal.Ip), offset
}

// terminalOf は端末の 4 列を端末の値にする。4 列がすべて NULL の行は端末を持たない。
func terminalOf(id, hostname, ip, offset sql.NullString) *Terminal {
	if !id.Valid && !hostname.Valid && !ip.Valid && !offset.Valid {
		return nil
	}
	terminal := &Terminal{Id: id.String, Hostname: hostname.String, Ip: ip.String}
	if offset.Valid {
		value := core.UtcOffset(offset.String)
		terminal.TimeOffset = &value
	}
	return terminal
}

// AddSources は取り込みの指定を記録した順の末尾へ足す。1 つの transaction で行う。
func (d *DB) AddSources(ctx context.Context, sources []Source) error {
	if err := d.transaction(ctx, func(tx *sql.Tx) error { return insertSources(ctx, tx, sources) }); err != nil {
		return fmt.Errorf("adding the sources to the investigation: %w", err)
	}
	return nil
}

func insertSources(ctx context.Context, tx *sql.Tx, sources []Source) error {
	for _, source := range sources {
		terminalId, terminalHostname, terminalIp, timeOffset := terminalColumns(source.Terminal)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO source (case_id, format_key, origin_path, format_spec, content_sha256, import_ordinal,
			terminal_id, terminal_hostname, terminal_ip, time_offset, collection_path)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			nullable(source.CaseId), string(source.FormatKey), source.OriginPath, nullable(source.FormatSpec),
			source.ContentSha256, source.ImportOrdinal,
			terminalId, terminalHostname, terminalIp, timeOffset,
			sql.NullString{String: source.CollectionPath, Valid: source.CollectionPath != ""}); err != nil {
			return fmt.Errorf("recording the source %q: %w", source.OriginPath, err)
		}
	}
	return nil
}

func (d *DB) readSources(ctx context.Context) ([]Source, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT case_id, format_key, origin_path, format_spec, content_sha256,
		import_ordinal, terminal_id, terminal_hostname, terminal_ip, time_offset, collection_path
		FROM source ORDER BY position`)
	if err != nil {
		return nil, fmt.Errorf("reading the sources: %w", err)
	}
	defer rows.Close()
	var sources []Source
	for rows.Next() {
		var source Source
		var caseId, formatSpec, terminalId, terminalHostname, terminalIp, timeOffset, collection sql.NullString
		var formatKey string
		if err := rows.Scan(&caseId, &formatKey, &source.OriginPath, &formatSpec, &source.ContentSha256,
			&source.ImportOrdinal, &terminalId, &terminalHostname, &terminalIp, &timeOffset, &collection); err != nil {
			return nil, fmt.Errorf("reading the sources: %w", err)
		}
		source.FormatKey, source.CollectionPath = core.FormatKey(formatKey), collection.String
		source.CaseId, source.FormatSpec = pointerOf(caseId), pointerOf(formatSpec)
		source.Terminal = terminalOf(terminalId, terminalHostname, terminalIp, timeOffset)
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the sources: %w", err)
	}
	return sources, nil
}

// AddSkippedFiles は、収集の directory にあり取り込まなかった file を記録した順の末尾へ足す。
// 同じ取得元の file を既に記録していれば、理由と種類を置き換え、並びの位置を保つ。
func (d *DB) AddSkippedFiles(ctx context.Context, files []core.SkippedFile) error {
	return d.transaction(ctx, func(tx *sql.Tx) error {
		if err := ensureAddedTables(ctx, tx); err != nil {
			return err
		}
		for _, file := range files {
			if _, err := tx.ExecContext(ctx, `INSERT INTO skipped_file (origin_path, reason, detected_kind)
				VALUES (?, ?, ?) ON CONFLICT (origin_path) DO UPDATE SET reason = excluded.reason,
					detected_kind = excluded.detected_kind`,
				file.OriginPath, string(file.Reason), file.DetectedKind); err != nil {
				return fmt.Errorf("recording the skipped file %q: %w", file.OriginPath, err)
			}
		}
		return nil
	})
}

func (d *DB) readSkippedFiles(ctx context.Context) ([]core.SkippedFile, error) {
	if exists, err := d.hasTable(ctx, "skipped_file"); err != nil || !exists {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx,
		`SELECT origin_path, reason, detected_kind FROM skipped_file ORDER BY position`)
	if err != nil {
		return nil, fmt.Errorf("reading the skipped files: %w", err)
	}
	defer rows.Close()
	var files []core.SkippedFile
	for rows.Next() {
		var file core.SkippedFile
		var reason string
		if err := rows.Scan(&file.OriginPath, &reason, &file.DetectedKind); err != nil {
			return nil, fmt.Errorf("reading the skipped files: %w", err)
		}
		file.Reason = core.SkippedFileReason(reason)
		if err := file.Validate(); err != nil {
			return nil, fmt.Errorf("reading the skipped files: %w", err)
		}
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the skipped files: %w", err)
	}
	return files, nil
}

// nullable は nil の pointer を SQL の NULL にする。
func nullable(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

// pointerOf は SQL の NULL を nil にする。空文字列の値は空文字列を指す pointer にする。
func pointerOf(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}
