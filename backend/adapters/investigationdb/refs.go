package investigationdb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// insertRecordRef はレコードの参照 1 件を record_ref の表へ書き、行の番号を返す。
func insertRecordRef(ctx context.Context, tx *sql.Tx, ref core.AssertionRecordRef) (int64, error) {
	result, err := tx.ExecContext(ctx, `INSERT INTO record_ref (source_content_sha256, position_kind,
		sequence_number, line_number, byte_offset) VALUES (?, ?, ?, ?, ?)`,
		ref.SourceContentSha256, string(ref.PositionKind), nullableInt(ref.SequenceNumber),
		nullableInt(ref.LineNumber), nullableInt(ref.ByteOffset))
	if err != nil {
		return 0, fmt.Errorf("recording the record reference: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("recording the record reference: %w", err)
	}
	return id, nil
}

// readRecordRefs は record_ref の表の全行を、行の番号から探せる形で読む。
func (d *DB) readRecordRefs(ctx context.Context) (map[int64]core.AssertionRecordRef, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT id, source_content_sha256, position_kind, sequence_number,
		line_number, byte_offset FROM record_ref`)
	if err != nil {
		return nil, fmt.Errorf("reading the record references: %w", err)
	}
	defer rows.Close()
	refs := map[int64]core.AssertionRecordRef{}
	for rows.Next() {
		var id int64
		var ref core.AssertionRecordRef
		var positionKind string
		var sequence, line, offset sql.NullInt64
		if err := rows.Scan(&id, &ref.SourceContentSha256, &positionKind, &sequence, &line, &offset); err != nil {
			return nil, fmt.Errorf("reading the record references: %w", err)
		}
		ref.PositionKind = core.PositionKind(positionKind)
		ref.SequenceNumber, ref.LineNumber, ref.ByteOffset = intPointerOf(sequence), intPointerOf(line),
			intPointerOf(offset)
		refs[id] = ref
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the record references: %w", err)
	}
	return refs, nil
}

// insertTimestamp は時刻 1 件を timestamp_value の表へ書き、行の番号を返す。
func insertTimestamp(ctx context.Context, tx *sql.Tx, value core.Timestamp) (int64, error) {
	result, err := tx.ExecContext(ctx, `INSERT INTO timestamp_value (raw_text, normalized, normalized_form,
		precision, offset_state, offset_text, clock, meaning, value_state) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nullable(value.RawText), nullable(value.Normalized), string(value.NormalizedForm), string(value.Precision),
		string(value.OffsetState), nullable(value.OffsetText), string(value.Clock), string(value.Meaning),
		string(value.ValueState))
	if err != nil {
		return 0, fmt.Errorf("recording the timestamp: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("recording the timestamp: %w", err)
	}
	return id, nil
}

// readTimestamps は timestamp_value の表の全行を、行の番号から探せる形で読む。
//
// **core.NewTimestamp を通して組む。** struct literal で組んだ値は関連付けに使う時刻を持たず、
// 読み戻した割当の期間が比べられなくなる。
func (d *DB) readTimestamps(ctx context.Context) (map[int64]core.Timestamp, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT id, raw_text, normalized, normalized_form, precision,
		offset_state, offset_text, clock, meaning, value_state FROM timestamp_value`)
	if err != nil {
		return nil, fmt.Errorf("reading the timestamps: %w", err)
	}
	defer rows.Close()
	values := map[int64]core.Timestamp{}
	for rows.Next() {
		var id int64
		var raw, normalized, offsetText sql.NullString
		var form, precision, offsetState, clock, meaning, valueState string
		if err := rows.Scan(&id, &raw, &normalized, &form, &precision, &offsetState, &offsetText, &clock,
			&meaning, &valueState); err != nil {
			return nil, fmt.Errorf("reading the timestamps: %w", err)
		}
		value, err := core.NewTimestamp(core.Timestamp{
			RawText: pointerOf(raw), Normalized: pointerOf(normalized), NormalizedForm: core.NormalizedForm(form),
			Precision: core.Precision(precision), OffsetState: core.OffsetState(offsetState),
			OffsetText: pointerOf(offsetText), Clock: core.Clock(clock), Meaning: core.Meaning(meaning),
			ValueState: core.ValueState(valueState),
		})
		if err != nil {
			return nil, fmt.Errorf("reading the timestamp %d: %w", id, err)
		}
		values[id] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the timestamps: %w", err)
	}
	return values, nil
}

func nullableInt(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func intPointerOf(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	number := value.Int64
	return &number
}

func emptyToNull(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}
