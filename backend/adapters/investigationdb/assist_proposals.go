package investigationdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// assistProposalSchema は AI 提案の表である。**表を持たない file も開ける。** 読むときは表が無ければ
// 空とし、書くときは同じ transaction で表を作る。
//
// **既存の表と列を変えない。** 採用で作った所見と提案の対応は、assertion の表に列を足さず、
// assist_proposal_assertion の表に置く。採用で作った所見の識別子もこの表から引く。
var assistProposalSchema = []string{
	`CREATE TABLE IF NOT EXISTS assist_proposal (
		position INTEGER PRIMARY KEY,
		id TEXT NOT NULL UNIQUE,
		ordinal INTEGER NOT NULL UNIQUE,
		target_kind TEXT NOT NULL,
		target_node_id TEXT,
		target_edge_kind TEXT,
		target_edge_source_node_id TEXT,
		target_edge_target_node_id TEXT,
		target_record_ref INTEGER REFERENCES record_ref (id),
		note TEXT NOT NULL,
		conversation_id TEXT NOT NULL,
		turn_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		model TEXT,
		adds_relation INTEGER NOT NULL CHECK (adds_relation IN (0, 1)),
		created_at TEXT NOT NULL,
		state TEXT NOT NULL CHECK (state IN ('proposed', 'adopted', 'rejected')),
		decided_by TEXT,
		decided_at TEXT,
		reason TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS assist_proposal_basis_ref (
		proposal INTEGER NOT NULL REFERENCES assist_proposal (position),
		element INTEGER NOT NULL,
		record_ref INTEGER NOT NULL REFERENCES record_ref (id),
		PRIMARY KEY (proposal, element)
	) WITHOUT ROWID`,
	`CREATE TABLE IF NOT EXISTS assist_proposal_match_condition (
		proposal INTEGER NOT NULL REFERENCES assist_proposal (position),
		element INTEGER NOT NULL,
		condition_key TEXT NOT NULL,
		tolerance INTEGER NOT NULL,
		PRIMARY KEY (proposal, element)
	) WITHOUT ROWID`,
	`CREATE TABLE IF NOT EXISTS assist_proposal_assertion (
		proposal INTEGER PRIMARY KEY REFERENCES assist_proposal (position),
		assertion INTEGER NOT NULL UNIQUE REFERENCES assertion (position)
	)`,
}

// ErrAssistProposalDecided は、採否を記録しようとした提案の状態が、file の上で既に提案中でないことを表す。
var ErrAssistProposalDecided = errors.New("investigation db: the assist proposal is already decided")

// StoredAssistProposal は記録した AI 提案 1 件と、その識別子を作った通番である。
type StoredAssistProposal struct {
	Proposal core.AssistProposal
	// Ordinal は提案を記録した通番である。次の提案の識別子を作る材料になる。
	Ordinal int64
}

// assistProposalTransaction は、AI 提案の表を作ってから body を同じ transaction で行う。
func (d *DB) assistProposalTransaction(ctx context.Context, body func(*sql.Tx) error) error {
	return d.transaction(ctx, func(tx *sql.Tx) error {
		for _, statement := range assistProposalSchema {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("creating the assist proposal tables: %w", err)
			}
		}
		return body(tx)
	})
}

// InsertAssistProposal は新しい AI 提案を、根拠と関連付けの条件の選択を含めて 1 つの transaction で書く。
func (d *DB) InsertAssistProposal(ctx context.Context, proposal core.AssistProposal, ordinal int64) error {
	err := d.assistProposalTransaction(ctx, func(tx *sql.Tx) error {
		target, err := targetColumnsOf(ctx, tx, proposal.Target)
		if err != nil {
			return err
		}
		decidedBy, decidedAt, reason := decisionColumnsOf(proposal.Decision)
		result, err := tx.ExecContext(ctx, `INSERT INTO assist_proposal (id, ordinal, target_kind, target_node_id,
			target_edge_kind, target_edge_source_node_id, target_edge_target_node_id, target_record_ref, note,
			conversation_id, turn_id, provider, model, adds_relation, created_at, state, decided_by, decided_at, reason)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, proposal.Id, ordinal,
			string(proposal.Target.Kind), target.nodeId, target.edgeKind, target.edgeSource, target.edgeTarget,
			target.recordRef, proposal.Note, proposal.ConversationId, proposal.TurnId, string(proposal.Provider),
			emptyToNull(proposal.Model), proposal.AddsRelation, string(proposal.CreatedAt), string(proposal.State),
			decidedBy, decidedAt, reason)
		if err != nil {
			return fmt.Errorf("recording the assist proposal: %w", err)
		}
		position, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("recording the assist proposal: %w", err)
		}
		return insertProposalElements(ctx, tx, position, proposal)
	})
	if err != nil {
		return fmt.Errorf("recording the assist proposal %q: %w", proposal.Id, err)
	}
	return nil
}

func insertProposalElements(ctx context.Context, tx *sql.Tx, position int64, proposal core.AssistProposal) error {
	for element, ref := range proposal.RecordRefs {
		id, err := insertRecordRef(ctx, tx, ref)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO assist_proposal_basis_ref (proposal, element, record_ref)
			VALUES (?, ?, ?)`, position, element, id); err != nil {
			return fmt.Errorf("recording the basis of the assist proposal: %w", err)
		}
	}
	for element, condition := range proposal.MatchConditions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO assist_proposal_match_condition (proposal, element,
			condition_key, tolerance) VALUES (?, ?, ?, ?)`, position, element, string(condition.ConditionKey),
			condition.Tolerance); err != nil {
			return fmt.Errorf("recording the match conditions of the assist proposal: %w", err)
		}
	}
	return nil
}

// AdoptAssistProposal は、提案を採用に変え、採用で作った所見と、所見と提案の対応を 1 つの transaction で
// 書く。proposal は採用の記録を持つ提案であり、assertion は assertionOrdinal の通番で作った所見である。
//
// file の上の提案が既に提案中でないときは ErrAssistProposalDecided を包んで返し、何も書かない。
func (d *DB) AdoptAssistProposal(
	ctx context.Context, proposal core.AssistProposal, assertion core.Assertion, assertionOrdinal int64,
) error {
	err := d.assistProposalTransaction(ctx, func(tx *sql.Tx) error {
		position, err := decideProposal(ctx, tx, proposal)
		if err != nil {
			return err
		}
		assertionPosition, err := insertAssertion(ctx, tx, assertion, assertionOrdinal)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO assist_proposal_assertion (proposal, assertion) VALUES (?, ?)`,
			position, assertionPosition); err != nil {
			return fmt.Errorf("recording the assertion adopted from the assist proposal: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("adopting the assist proposal %q: %w", proposal.Id, err)
	}
	return nil
}

// RejectAssistProposal は提案を却下に変える。file の上の提案が既に提案中でないときは
// ErrAssistProposalDecided を包んで返し、何も書かない。
func (d *DB) RejectAssistProposal(ctx context.Context, proposal core.AssistProposal) error {
	err := d.assistProposalTransaction(ctx, func(tx *sql.Tx) error {
		_, err := decideProposal(ctx, tx, proposal)
		return err
	})
	if err != nil {
		return fmt.Errorf("rejecting the assist proposal %q: %w", proposal.Id, err)
	}
	return nil
}

// decideProposal は提案中の提案の状態と採否の記録を tx の中で書き換え、提案の行の番号を返す。
//
// **状態が提案中の行だけを書き換える。** 2 人が同じ提案の採否を決めたとき、2 件目は行を書き換えず、
// ErrAssistProposalDecided になる。
func decideProposal(ctx context.Context, tx *sql.Tx, proposal core.AssistProposal) (int64, error) {
	decidedBy, decidedAt, reason := decisionColumnsOf(proposal.Decision)
	result, err := tx.ExecContext(ctx, `UPDATE assist_proposal SET state = ?, decided_by = ?, decided_at = ?,
		reason = ? WHERE id = ? AND state = ?`, string(proposal.State), decidedBy, decidedAt, reason, proposal.Id,
		string(core.AssistProposalStateProposed))
	if err != nil {
		return 0, fmt.Errorf("recording the decision: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("recording the decision: %w", err)
	}
	if changed != 1 {
		return 0, ErrAssistProposalDecided
	}
	var position int64
	if err := tx.QueryRowContext(ctx, `SELECT position FROM assist_proposal WHERE id = ?`,
		proposal.Id).Scan(&position); err != nil {
		return 0, fmt.Errorf("finding the assist proposal: %w", err)
	}
	return position, nil
}

// decisionColumnsOf は採否の記録を列の値にする。提案中の提案では 3 つとも NULL になる。
func decisionColumnsOf(decision *core.AssistProposalDecision) (decidedBy, decidedAt, reason sql.NullString) {
	if decision == nil {
		return sql.NullString{}, sql.NullString{}, sql.NullString{}
	}
	return sql.NullString{String: decision.Analyst, Valid: true},
		sql.NullString{String: string(decision.DecidedAt), Valid: true}, emptyToNull(decision.Reason)
}

// readAdoptedProposalIds は、採用で作った所見の行の番号から元の提案の識別子を引く対応を読む。
// 表を持たない file では空である。
func (d *DB) readAdoptedProposalIds(ctx context.Context) (map[int64]string, error) {
	if exists, err := d.hasTable(ctx, "assist_proposal_assertion"); err != nil || !exists {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT link.assertion, proposal.id FROM assist_proposal_assertion AS link
		JOIN assist_proposal AS proposal ON proposal.position = link.proposal`)
	if err != nil {
		return nil, fmt.Errorf("reading the adopted assist proposals: %w", err)
	}
	defer rows.Close()
	ids := map[int64]string{}
	for rows.Next() {
		var assertion int64
		var id string
		if err := rows.Scan(&assertion, &id); err != nil {
			return nil, fmt.Errorf("reading the adopted assist proposals: %w", err)
		}
		ids[assertion] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the adopted assist proposals: %w", err)
	}
	return ids, nil
}

// readAssistProposals は AI 提案を記録した順に読み、core の Validate を通して返す。表を持たない file
// では空である。
func (d *DB) readAssistProposals(ctx context.Context) ([]StoredAssistProposal, error) {
	if exists, err := d.hasTable(ctx, "assist_proposal"); err != nil || !exists {
		return nil, err
	}
	refs, err := d.readRecordRefs(ctx)
	if err != nil {
		return nil, err
	}
	basis, err := d.readProposalBasis(ctx, refs)
	if err != nil {
		return nil, err
	}
	conditions, err := d.readProposalConditions(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT proposal.position, proposal.id, proposal.ordinal,
		proposal.target_kind, proposal.target_node_id, proposal.target_edge_kind,
		proposal.target_edge_source_node_id, proposal.target_edge_target_node_id, proposal.target_record_ref,
		proposal.note, proposal.conversation_id, proposal.turn_id, proposal.provider, proposal.model,
		proposal.adds_relation, proposal.created_at, proposal.state, proposal.decided_by, proposal.decided_at,
		proposal.reason, assertion.id
		FROM assist_proposal AS proposal
		LEFT JOIN assist_proposal_assertion AS link ON link.proposal = proposal.position
		LEFT JOIN assertion ON assertion.position = link.assertion
		ORDER BY proposal.position`)
	if err != nil {
		return nil, fmt.Errorf("reading the assist proposals: %w", err)
	}
	defer rows.Close()
	var proposals []StoredAssistProposal
	for rows.Next() {
		stored, position, err := scanProposal(rows, refs)
		if err != nil {
			return nil, err
		}
		stored.Proposal.RecordRefs, stored.Proposal.MatchConditions = basis[position], conditions[position]
		if err := stored.Proposal.Validate(); err != nil {
			return nil, fmt.Errorf("reading the assist proposal %q: %w", stored.Proposal.Id, err)
		}
		proposals = append(proposals, stored)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the assist proposals: %w", err)
	}
	return proposals, nil
}

// scanProposal は提案の行 1 つを、根拠と関連付けの条件を除いて読む。提案の行の番号を一緒に返す。
func scanProposal(rows *sql.Rows, refs map[int64]core.AssertionRecordRef) (StoredAssistProposal, int64, error) {
	var position int64
	var stored StoredAssistProposal
	var kind, provider, createdAt, state string
	var nodeId, edgeKind, edgeSource, edgeTarget, model, decidedBy, decidedAt, reason, assertionId sql.NullString
	var recordRef sql.NullInt64
	proposal := &stored.Proposal
	if err := rows.Scan(&position, &proposal.Id, &stored.Ordinal, &kind, &nodeId, &edgeKind, &edgeSource,
		&edgeTarget, &recordRef, &proposal.Note, &proposal.ConversationId, &proposal.TurnId, &provider, &model,
		&proposal.AddsRelation, &createdAt, &state, &decidedBy, &decidedAt, &reason, &assertionId); err != nil {
		return StoredAssistProposal{}, 0, fmt.Errorf("reading the assist proposals: %w", err)
	}
	proposal.Target = core.AssertionTarget{Kind: core.AssertionTargetKind(kind), NodeId: nodeId.String}
	if edgeKind.Valid {
		proposal.Target.Edge = &core.AssertionEdgeRef{
			Kind: core.EdgeKind(edgeKind.String), SourceNodeId: edgeSource.String, TargetNodeId: edgeTarget.String,
		}
	}
	if recordRef.Valid {
		ref, found := refs[recordRef.Int64]
		if !found {
			return StoredAssistProposal{}, 0, fmt.Errorf("reading the assist proposal %q: "+
				"the target record reference is missing", proposal.Id)
		}
		proposal.Target.Record = &ref
	}
	proposal.Provider, proposal.Model = core.AssistProvider(provider), model.String
	proposal.CreatedAt, proposal.State = core.AssertionTime(createdAt), core.AssistProposalState(state)
	if decidedBy.Valid {
		proposal.Decision = &core.AssistProposalDecision{
			Analyst: decidedBy.String, DecidedAt: core.AssertionTime(decidedAt.String), Reason: reason.String,
			AssertionId: assertionId.String,
		}
	}
	return stored, position, nil
}

// readProposalBasis は提案の根拠のレコードを、提案の行の番号ごとに並びの順で読む。
func (d *DB) readProposalBasis(
	ctx context.Context, refs map[int64]core.AssertionRecordRef,
) (map[int64][]core.AssertionRecordRef, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT proposal, record_ref FROM assist_proposal_basis_ref
		ORDER BY proposal, element`)
	if err != nil {
		return nil, fmt.Errorf("reading the assist proposal bases: %w", err)
	}
	defer rows.Close()
	basis := map[int64][]core.AssertionRecordRef{}
	for rows.Next() {
		var position, id int64
		if err := rows.Scan(&position, &id); err != nil {
			return nil, fmt.Errorf("reading the assist proposal bases: %w", err)
		}
		ref, found := refs[id]
		if !found {
			return nil, errors.New("reading the assist proposal bases: a basis record reference is missing")
		}
		basis[position] = append(basis[position], ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the assist proposal bases: %w", err)
	}
	return basis, nil
}

// readProposalConditions は提案を作った発言の関連付けの条件を、提案の行の番号ごとに並びの順で読む。
func (d *DB) readProposalConditions(ctx context.Context) (map[int64][]core.AssistMatchCondition, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT proposal, condition_key, tolerance
		FROM assist_proposal_match_condition ORDER BY proposal, element`)
	if err != nil {
		return nil, fmt.Errorf("reading the assist proposal match conditions: %w", err)
	}
	defer rows.Close()
	conditions := map[int64][]core.AssistMatchCondition{}
	for rows.Next() {
		var position int64
		var condition core.AssistMatchCondition
		var key string
		if err := rows.Scan(&position, &key, &condition.Tolerance); err != nil {
			return nil, fmt.Errorf("reading the assist proposal match conditions: %w", err)
		}
		condition.ConditionKey = core.ConditionKey(key)
		conditions[position] = append(conditions[position], condition)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the assist proposal match conditions: %w", err)
	}
	return conditions, nil
}
