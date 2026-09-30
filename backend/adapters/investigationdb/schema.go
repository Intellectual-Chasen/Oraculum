package investigationdb

import (
	"errors"

	sqlite "modernc.org/sqlite"
)

// schema は調査の file の表である。Create が 1 つの transaction で作る。
//
// **本 package が読み書きしない表を、開いた file が持っていても読み飛ばす。** 表を消す変更の前に
// 作った file は、その表を持ったまま開ける。表を DROP しない。
//
// **DB の列名を core の意味として固定しない。** 列と core の型の対応は本 package の読み書きの
// 関数だけが持つ。
var schema = []string{
	`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL) WITHOUT ROWID`,
	`CREATE TABLE source (
		position INTEGER PRIMARY KEY,
		case_id TEXT,
		format_key TEXT NOT NULL,
		origin_path TEXT NOT NULL,
		format_spec TEXT,
		content_sha256 TEXT NOT NULL,
		import_ordinal INTEGER NOT NULL,
		terminal_id TEXT,
		terminal_hostname TEXT,
		terminal_ip TEXT,
		time_offset TEXT,
		collection_path TEXT,
		UNIQUE (origin_path, content_sha256, import_ordinal)
	)`,
	`CREATE TABLE assertion (
		position INTEGER PRIMARY KEY,
		id TEXT NOT NULL UNIQUE,
		ordinal INTEGER NOT NULL UNIQUE,
		target_kind TEXT NOT NULL,
		target_node_id TEXT,
		target_edge_kind TEXT,
		target_edge_source_node_id TEXT,
		target_edge_target_node_id TEXT,
		target_record_ref INTEGER REFERENCES record_ref (id),
		target_source_content_sha256 TEXT,
		adds_relation INTEGER NOT NULL CHECK (adds_relation IN (0, 1))
	)`,
	`CREATE TABLE assertion_revision (
		assertion INTEGER NOT NULL REFERENCES assertion (position),
		revision_number INTEGER NOT NULL,
		state TEXT NOT NULL,
		author TEXT NOT NULL,
		recorded_at TEXT NOT NULL,
		note TEXT NOT NULL,
		time_offset TEXT,
		PRIMARY KEY (assertion, revision_number)
	) WITHOUT ROWID`,
	`CREATE TABLE assertion_basis_ref (
		assertion INTEGER NOT NULL,
		revision_number INTEGER NOT NULL,
		element INTEGER NOT NULL,
		record_ref INTEGER NOT NULL REFERENCES record_ref (id),
		PRIMARY KEY (assertion, revision_number, element),
		FOREIGN KEY (assertion, revision_number) REFERENCES assertion_revision (assertion, revision_number)
	) WITHOUT ROWID`,
	`CREATE TABLE terminal_assignment (
		position INTEGER PRIMARY KEY,
		client_ip TEXT NOT NULL,
		terminal_id TEXT NOT NULL,
		terminal_hostname TEXT NOT NULL,
		terminal_hostnames TEXT NOT NULL,
		source_id TEXT NOT NULL,
		source_content_sha256 TEXT NOT NULL,
		valid_from INTEGER NOT NULL REFERENCES timestamp_value (id),
		valid_to INTEGER NOT NULL REFERENCES timestamp_value (id),
		origin TEXT NOT NULL,
		derivation TEXT NOT NULL,
		author TEXT NOT NULL,
		applies_to_source_id TEXT NOT NULL
	)`,
	`CREATE TABLE terminal_assignment_basis_ref (
		assignment INTEGER NOT NULL REFERENCES terminal_assignment (position),
		element INTEGER NOT NULL,
		record_ref INTEGER NOT NULL REFERENCES record_ref (id),
		PRIMARY KEY (assignment, element)
	) WITHOUT ROWID`,
	`CREATE TABLE record_ref (
		id INTEGER PRIMARY KEY,
		source_content_sha256 TEXT NOT NULL,
		position_kind TEXT NOT NULL,
		sequence_number INTEGER,
		line_number INTEGER,
		byte_offset INTEGER
	)`,
	`CREATE TABLE timestamp_value (
		id INTEGER PRIMARY KEY,
		raw_text TEXT,
		normalized TEXT,
		normalized_form TEXT NOT NULL,
		precision TEXT NOT NULL,
		offset_state TEXT NOT NULL,
		offset_text TEXT,
		clock TEXT NOT NULL,
		meaning TEXT NOT NULL,
		value_state TEXT NOT NULL
	)`,
}

// sqliteBusy と sqliteLocked は SQLite の結果コードの主の値である。
const (
	sqliteBusy   = 5
	sqliteLocked = 6
)

// isBusy は err が、別の接続が lock を握っていることによる失敗かを返す。
func isBusy(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	primary := sqliteErr.Code() & 0xff
	return primary == sqliteBusy || primary == sqliteLocked
}
