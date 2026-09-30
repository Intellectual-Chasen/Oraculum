package investigationdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// assistConversationSchema は、会話、発言、短い参照、受け渡しの記録の表である。assistSchema と
// 同じ transaction で作る。
var assistConversationSchema = []string{
	`CREATE TABLE IF NOT EXISTS assist_conversation (
		position INTEGER PRIMARY KEY,
		id TEXT NOT NULL UNIQUE,
		provider TEXT NOT NULL,
		model TEXT,
		permission_revision INTEGER NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS assist_turn (
		conversation TEXT NOT NULL REFERENCES assist_conversation (id),
		turn_id TEXT NOT NULL,
		case_id TEXT,
		PRIMARY KEY (conversation, turn_id)
	) WITHOUT ROWID`,
	`CREATE TABLE IF NOT EXISTS assist_turn_condition (
		conversation TEXT NOT NULL,
		turn_id TEXT NOT NULL,
		element INTEGER NOT NULL,
		condition_key TEXT NOT NULL,
		tolerance INTEGER NOT NULL,
		PRIMARY KEY (conversation, turn_id, element),
		FOREIGN KEY (conversation, turn_id) REFERENCES assist_turn (conversation, turn_id)
	) WITHOUT ROWID`,
	`CREATE TABLE IF NOT EXISTS assist_short_ref (
		conversation TEXT NOT NULL REFERENCES assist_conversation (id),
		ordinal INTEGER NOT NULL,
		ref TEXT NOT NULL,
		kind TEXT NOT NULL CHECK (kind IN ('record', 'node', 'edge')),
		source_id TEXT,
		record_ref INTEGER REFERENCES record_ref (id),
		node_id TEXT,
		edge_id TEXT,
		PRIMARY KEY (conversation, ref)
	) WITHOUT ROWID`,
	`CREATE TABLE IF NOT EXISTS assist_disclosure (
		position INTEGER PRIMARY KEY,
		conversation TEXT NOT NULL REFERENCES assist_conversation (id),
		turn_id TEXT NOT NULL,
		tool TEXT NOT NULL,
		request TEXT NOT NULL,
		truncated INTEGER NOT NULL CHECK (truncated IN (0, 1)),
		body_sha256 TEXT NOT NULL,
		body_bytes INTEGER NOT NULL,
		server_revision TEXT,
		server_modified INTEGER NOT NULL CHECK (server_modified IN (0, 1)),
		assignment_revision INTEGER NOT NULL,
		interpretation_revision INTEGER NOT NULL,
		source_set_sha256 TEXT NOT NULL,
		permission_revision INTEGER NOT NULL,
		recorded_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS assist_disclosure_condition (
		disclosure INTEGER NOT NULL REFERENCES assist_disclosure (position),
		element INTEGER NOT NULL,
		condition_key TEXT NOT NULL,
		tolerance INTEGER NOT NULL,
		PRIMARY KEY (disclosure, element)
	) WITHOUT ROWID`,
	`CREATE TABLE IF NOT EXISTS assist_disclosure_record (
		disclosure INTEGER NOT NULL REFERENCES assist_disclosure (position),
		element INTEGER NOT NULL,
		short_ref TEXT NOT NULL,
		PRIMARY KEY (disclosure, element)
	) WITHOUT ROWID`,
}

// AssistConversations は調査の file が持つ会話と、その発言、短い参照、受け渡しの記録である。
type AssistConversations struct {
	// Conversations は会話を発行した順に並べる。
	Conversations []core.AssistConversation
	// Turns は発言を会話ごとに登録した順に並べる。
	Turns []core.AssistTurn
	// Refs は会話の識別子から、その会話で発行した短い参照を発行した順に取り出す。
	Refs map[string][]core.AssistShortRef
	// Disclosures は受け渡しの記録を記録した順に並べる。
	Disclosures []core.AssistDisclosure
}

// InsertAssistConversation は発行した会話 1 件を、1 つの transaction で書く。
func (d *DB) InsertAssistConversation(ctx context.Context, conversation core.AssistConversation) error {
	err := d.assistTransaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO assist_conversation (id, provider, model, permission_revision,
			created_at) VALUES (?, ?, ?, ?, ?)`, conversation.Id, string(conversation.Provider),
			emptyToNull(conversation.Model), conversation.PermissionRevision,
			string(conversation.CreatedAt)); err != nil {
			return fmt.Errorf("recording the conversation: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("recording the assist conversation: %w", err)
	}
	return nil
}

// InsertAssistDisclosure は、受け渡しの記録と、同じ受け渡しで登録した発言と発行した短い参照を、
// 1 つの transaction で書く。turn は発言を登録しない受け渡しでは nil である。
func (d *DB) InsertAssistDisclosure(
	ctx context.Context, disclosure core.AssistDisclosure, turn *core.AssistTurn, refs []core.AssistShortRef,
) error {
	err := d.assistTransaction(ctx, func(tx *sql.Tx) error {
		if turn != nil {
			if err := insertAssistTurn(ctx, tx, *turn); err != nil {
				return err
			}
		}
		for _, ref := range refs {
			if err := insertAssistShortRef(ctx, tx, disclosure.ConversationId, ref); err != nil {
				return err
			}
		}
		return insertAssistDisclosure(ctx, tx, disclosure)
	})
	if err != nil {
		// 要求と本文の文字列を error に載せない。呼び出し元が log へ出す。
		return fmt.Errorf("recording the assist disclosure: %w", err)
	}
	return nil
}

func insertAssistTurn(ctx context.Context, tx *sql.Tx, turn core.AssistTurn) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO assist_turn (conversation, turn_id, case_id) VALUES (?, ?, ?)`,
		turn.ConversationId, turn.TurnId, emptyToNull(turn.Case)); err != nil {
		return fmt.Errorf("recording the turn: %w", err)
	}
	for element, condition := range turn.MatchConditions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO assist_turn_condition (conversation, turn_id, element,
			condition_key, tolerance) VALUES (?, ?, ?, ?, ?)`, turn.ConversationId, turn.TurnId, element,
			string(condition.ConditionKey), condition.Tolerance); err != nil {
			return fmt.Errorf("recording the turn conditions: %w", err)
		}
	}
	return nil
}

func insertAssistShortRef(ctx context.Context, tx *sql.Tx, conversationId string, ref core.AssistShortRef) error {
	var sourceId, nodeId, edgeId sql.NullString
	var recordRef sql.NullInt64
	switch ref.Kind {
	case core.AssistRefKindRecord:
		id, err := insertRecordRef(ctx, tx, ref.Record.Record)
		if err != nil {
			return err
		}
		sourceId, recordRef = emptyToNull(ref.Record.SourceId), sql.NullInt64{Int64: id, Valid: true}
	case core.AssistRefKindNode:
		nodeId = emptyToNull(ref.NodeId)
	case core.AssistRefKindEdge:
		edgeId = emptyToNull(ref.EdgeId)
	}
	var ordinal int64
	if err := tx.QueryRowContext(ctx, `SELECT count(*) + 1 FROM assist_short_ref WHERE conversation = ? AND kind = ?`,
		conversationId, string(ref.Kind)).Scan(&ordinal); err != nil {
		return fmt.Errorf("recording the short reference: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO assist_short_ref (conversation, ordinal, ref, kind, source_id,
		record_ref, node_id, edge_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, conversationId, ordinal, ref.Ref,
		string(ref.Kind), sourceId, recordRef, nodeId, edgeId); err != nil {
		return fmt.Errorf("recording the short reference: %w", err)
	}
	return nil
}

func insertAssistDisclosure(ctx context.Context, tx *sql.Tx, disclosure core.AssistDisclosure) error {
	versions := disclosure.Versions
	if _, err := tx.ExecContext(ctx, `INSERT INTO assist_disclosure (position, conversation, turn_id, tool, request,
		truncated, body_sha256, body_bytes, server_revision, server_modified, assignment_revision,
		interpretation_revision, source_set_sha256, permission_revision, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, disclosure.Ordinal, disclosure.ConversationId,
		disclosure.TurnId, string(disclosure.Tool), disclosure.Request, disclosure.Truncated, disclosure.BodySha256,
		disclosure.BodyBytes, emptyToNull(versions.ServerRevision), versions.ServerModified,
		versions.AssignmentRevision, versions.InterpretationRevision, versions.SourceSetSha256,
		versions.PermissionRevision, string(disclosure.RecordedAt)); err != nil {
		return fmt.Errorf("recording the disclosure: %w", err)
	}
	for element, condition := range versions.MatchConditions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO assist_disclosure_condition (disclosure, element,
			condition_key, tolerance) VALUES (?, ?, ?, ?)`, disclosure.Ordinal, element,
			string(condition.ConditionKey), condition.Tolerance); err != nil {
			return fmt.Errorf("recording the disclosure conditions: %w", err)
		}
	}
	for element, ref := range disclosure.RecordRefs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO assist_disclosure_record (disclosure, element, short_ref)
			VALUES (?, ?, ?)`, disclosure.Ordinal, element, ref); err != nil {
			return fmt.Errorf("recording the disclosed records: %w", err)
		}
	}
	return nil
}

// readAssistConversations は会話と、その発言、短い参照、受け渡しの記録を読み、core の Validate を
// 通して返す。表を持たない file では空を返す。
func (d *DB) readAssistConversations(ctx context.Context) (AssistConversations, error) {
	exists, err := d.hasTable(ctx, "assist_conversation")
	if err != nil || !exists {
		return AssistConversations{}, err
	}
	var read AssistConversations
	if read.Conversations, err = d.readConversationRows(ctx); err != nil {
		return AssistConversations{}, err
	}
	if read.Turns, err = d.readTurnRows(ctx); err != nil {
		return AssistConversations{}, err
	}
	if read.Refs, err = d.readShortRefRows(ctx); err != nil {
		return AssistConversations{}, err
	}
	if read.Disclosures, err = d.readDisclosureRows(ctx); err != nil {
		return AssistConversations{}, err
	}
	return read, nil
}

func (d *DB) readConversationRows(ctx context.Context) ([]core.AssistConversation, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT id, provider, model, permission_revision, created_at
		FROM assist_conversation ORDER BY position`)
	if err != nil {
		return nil, fmt.Errorf("reading the assist conversations: %w", err)
	}
	defer rows.Close()
	var conversations []core.AssistConversation
	for rows.Next() {
		var conversation core.AssistConversation
		var provider, createdAt string
		var model sql.NullString
		if err := rows.Scan(&conversation.Id, &provider, &model, &conversation.PermissionRevision,
			&createdAt); err != nil {
			return nil, fmt.Errorf("reading the assist conversations: %w", err)
		}
		conversation.Provider = core.AssistProvider(provider)
		conversation.Model = model.String
		conversation.CreatedAt = core.AssertionTime(createdAt)
		if err := conversation.Validate(); err != nil {
			return nil, fmt.Errorf("reading an assist conversation: %w", err)
		}
		conversations = append(conversations, conversation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the assist conversations: %w", err)
	}
	return conversations, nil
}

func (d *DB) readTurnRows(ctx context.Context) ([]core.AssistTurn, error) {
	conditions, err := d.readConditionRows(ctx, `SELECT conversation || char(0) || turn_id, condition_key, tolerance
		FROM assist_turn_condition ORDER BY conversation, turn_id, element`)
	if err != nil {
		return nil, fmt.Errorf("reading the turn conditions: %w", err)
	}
	rows, err := d.db.QueryContext(ctx, `SELECT conversation, turn_id, case_id FROM assist_turn`)
	if err != nil {
		return nil, fmt.Errorf("reading the assist turns: %w", err)
	}
	defer rows.Close()
	var turns []core.AssistTurn
	for rows.Next() {
		var turn core.AssistTurn
		var caseId sql.NullString
		if err := rows.Scan(&turn.ConversationId, &turn.TurnId, &caseId); err != nil {
			return nil, fmt.Errorf("reading the assist turns: %w", err)
		}
		turn.Case = caseId.String
		turn.MatchConditions = conditions[turn.ConversationId+"\x00"+turn.TurnId]
		if turn.MatchConditions == nil {
			turn.MatchConditions = []core.AssistMatchCondition{}
		}
		if err := turn.Validate(); err != nil {
			return nil, fmt.Errorf("reading an assist turn: %w", err)
		}
		turns = append(turns, turn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the assist turns: %w", err)
	}
	return turns, nil
}

// readConditionRows は、鍵、条件の種別、幅の 3 列を返す query を読み、鍵ごとに条件を並べる。
func (d *DB) readConditionRows(ctx context.Context, query string) (map[string][]core.AssistMatchCondition, error) {
	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	conditions := map[string][]core.AssistMatchCondition{}
	for rows.Next() {
		var key, conditionKey string
		var tolerance int64
		if err := rows.Scan(&key, &conditionKey, &tolerance); err != nil {
			return nil, err
		}
		conditions[key] = append(conditions[key], core.AssistMatchCondition{
			ConditionKey: core.ConditionKey(conditionKey), Tolerance: tolerance,
		})
	}
	return conditions, rows.Err()
}

func (d *DB) readShortRefRows(ctx context.Context) (map[string][]core.AssistShortRef, error) {
	recordRefs, err := d.readRecordRefs(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT conversation, ref, kind, source_id, record_ref, node_id, edge_id
		FROM assist_short_ref ORDER BY conversation, kind, ordinal`)
	if err != nil {
		return nil, fmt.Errorf("reading the short references: %w", err)
	}
	defer rows.Close()
	refs := map[string][]core.AssistShortRef{}
	for rows.Next() {
		var conversation, kind string
		var ref core.AssistShortRef
		var sourceId, nodeId, edgeId sql.NullString
		var recordRef sql.NullInt64
		if err := rows.Scan(&conversation, &ref.Ref, &kind, &sourceId, &recordRef, &nodeId, &edgeId); err != nil {
			return nil, fmt.Errorf("reading the short references: %w", err)
		}
		ref.Kind = core.AssistRefKind(kind)
		ref.NodeId, ref.EdgeId = nodeId.String, edgeId.String
		if recordRef.Valid {
			record, found := recordRefs[recordRef.Int64]
			if !found {
				return nil, errors.New("reading the short references: a record reference is missing")
			}
			ref.Record = &core.AssistRecordTarget{SourceId: sourceId.String, Record: record}
		}
		if err := ref.Validate(); err != nil {
			return nil, fmt.Errorf("reading a short reference: %w", err)
		}
		refs[conversation] = append(refs[conversation], ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the short references: %w", err)
	}
	return refs, nil
}

func (d *DB) readDisclosureRows(ctx context.Context) ([]core.AssistDisclosure, error) {
	conditions, err := d.readConditionRows(ctx, `SELECT CAST(disclosure AS TEXT), condition_key, tolerance
		FROM assist_disclosure_condition ORDER BY disclosure, element`)
	if err != nil {
		return nil, fmt.Errorf("reading the disclosure conditions: %w", err)
	}
	records, err := d.readDisclosedRecords(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT position, conversation, turn_id, tool, request, truncated,
		body_sha256, body_bytes, server_revision, server_modified, assignment_revision, interpretation_revision,
		source_set_sha256, permission_revision, recorded_at FROM assist_disclosure ORDER BY position`)
	if err != nil {
		return nil, fmt.Errorf("reading the assist disclosures: %w", err)
	}
	defer rows.Close()
	var disclosures []core.AssistDisclosure
	for rows.Next() {
		var disclosure core.AssistDisclosure
		var tool, recordedAt string
		var serverRevision sql.NullString
		versions := &disclosure.Versions
		if err := rows.Scan(&disclosure.Ordinal, &disclosure.ConversationId, &disclosure.TurnId, &tool,
			&disclosure.Request, &disclosure.Truncated, &disclosure.BodySha256, &disclosure.BodyBytes,
			&serverRevision, &versions.ServerModified, &versions.AssignmentRevision,
			&versions.InterpretationRevision, &versions.SourceSetSha256, &versions.PermissionRevision,
			&recordedAt); err != nil {
			return nil, fmt.Errorf("reading the assist disclosures: %w", err)
		}
		disclosure.Tool = core.AssistTool(tool)
		disclosure.RecordedAt = core.AssertionTime(recordedAt)
		versions.ServerRevision = serverRevision.String
		versions.MatchConditions = conditions[fmt.Sprint(disclosure.Ordinal)]
		if versions.MatchConditions == nil {
			versions.MatchConditions = []core.AssistMatchCondition{}
		}
		disclosure.RecordRefs = records[disclosure.Ordinal]
		if disclosure.RecordRefs == nil {
			disclosure.RecordRefs = []string{}
		}
		if err := disclosure.Validate(); err != nil {
			return nil, fmt.Errorf("reading the assist disclosure %d: %w", disclosure.Ordinal, err)
		}
		disclosures = append(disclosures, disclosure)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the assist disclosures: %w", err)
	}
	return disclosures, nil
}

func (d *DB) readDisclosedRecords(ctx context.Context) (map[int64][]string, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT disclosure, short_ref FROM assist_disclosure_record
		ORDER BY disclosure, element`)
	if err != nil {
		return nil, fmt.Errorf("reading the disclosed records: %w", err)
	}
	defer rows.Close()
	records := map[int64][]string{}
	for rows.Next() {
		var disclosure int64
		var ref string
		if err := rows.Scan(&disclosure, &ref); err != nil {
			return nil, fmt.Errorf("reading the disclosed records: %w", err)
		}
		records[disclosure] = append(records[disclosure], ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the disclosed records: %w", err)
	}
	return records, nil
}
