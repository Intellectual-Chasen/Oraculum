package investigationdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// StoredAssertion は記録した所見 1 件と、その識別子を作った通番である。
type StoredAssertion struct {
	Assertion core.Assertion
	// Ordinal は所見を記録した通番である。次の所見の識別子を作る材料になる。
	Ordinal int64
}

// InsertAssertion は新しい所見を、履歴と現在の改訂を含めて 1 つの transaction で書く。
func (d *DB) InsertAssertion(ctx context.Context, assertion core.Assertion, ordinal int64) error {
	err := d.transaction(ctx, func(tx *sql.Tx) error {
		_, err := insertAssertion(ctx, tx, assertion, ordinal)
		return err
	})
	if err != nil {
		return fmt.Errorf("recording the assertion %q: %w", assertion.Id, err)
	}
	return nil
}

// insertAssertion は所見の行と全改訂を tx の中で書き、所見の行の番号を返す。
func insertAssertion(ctx context.Context, tx *sql.Tx, assertion core.Assertion, ordinal int64) (int64, error) {
	target, err := targetColumnsOf(ctx, tx, assertion.Target)
	if err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO assertion (id, ordinal, target_kind, target_node_id,
		target_edge_kind, target_edge_source_node_id, target_edge_target_node_id, target_record_ref,
		target_source_content_sha256, adds_relation)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, assertion.Id, ordinal, string(assertion.Target.Kind),
		target.nodeId, target.edgeKind, target.edgeSource, target.edgeTarget, target.recordRef,
		target.sourceContent, assertion.AddsRelation)
	if err != nil {
		return 0, fmt.Errorf("recording the assertion: %w", err)
	}
	position, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("recording the assertion: %w", err)
	}
	for _, revision := range revisionsOf(assertion) {
		if err := insertRevision(ctx, tx, position, revision); err != nil {
			return 0, err
		}
	}
	return position, nil
}

// ReviseAssertion は既存の所見へ、現在の改訂 1 件を足す。置き換えられた改訂は既に書いてある。
func (d *DB) ReviseAssertion(ctx context.Context, assertion core.Assertion) error {
	err := d.transaction(ctx, func(tx *sql.Tx) error {
		var position int64
		if err := tx.QueryRowContext(ctx, `SELECT position FROM assertion WHERE id = ?`,
			assertion.Id).Scan(&position); err != nil {
			return fmt.Errorf("finding the assertion: %w", err)
		}
		return insertRevision(ctx, tx, position, currentRevisionOf(assertion))
	})
	if err != nil {
		return fmt.Errorf("recording the revision %d of the assertion %q: %w",
			assertion.RevisionNumber, assertion.Id, err)
	}
	return nil
}

// targetColumns は所見の対象を assertion の表の列にした値である。
type targetColumns struct {
	nodeId, edgeKind, edgeSource, edgeTarget, sourceContent sql.NullString
	recordRef                                               sql.NullInt64
}

func targetColumnsOf(ctx context.Context, tx *sql.Tx, target core.AssertionTarget) (targetColumns, error) {
	columns := targetColumns{
		nodeId: emptyToNull(target.NodeId), sourceContent: emptyToNull(target.SourceContentSha256),
	}
	if target.Edge != nil {
		columns.edgeKind = sql.NullString{String: string(target.Edge.Kind), Valid: true}
		columns.edgeSource = sql.NullString{String: target.Edge.SourceNodeId, Valid: true}
		columns.edgeTarget = sql.NullString{String: target.Edge.TargetNodeId, Valid: true}
	}
	if target.Record != nil {
		id, err := insertRecordRef(ctx, tx, *target.Record)
		if err != nil {
			return targetColumns{}, err
		}
		columns.recordRef = sql.NullInt64{Int64: id, Valid: true}
	}
	return columns, nil
}

// revisionsOf は所見の全改訂を、改訂の番号の昇順に並べる。末尾が現在の改訂である。
func revisionsOf(assertion core.Assertion) []core.AssertionRevision {
	return append(slices.Clone(assertion.History), currentRevisionOf(assertion))
}

func currentRevisionOf(assertion core.Assertion) core.AssertionRevision {
	return core.AssertionRevision{
		RevisionNumber: assertion.RevisionNumber, State: assertion.State, Author: assertion.Author,
		RecordedAt: assertion.RecordedAt, Basis: assertion.Basis, TimeOffset: assertion.TimeOffset,
	}
}

func insertRevision(ctx context.Context, tx *sql.Tx, position int64, revision core.AssertionRevision) error {
	var offset sql.NullString
	if revision.TimeOffset != nil {
		offset = sql.NullString{String: string(*revision.TimeOffset), Valid: true}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO assertion_revision (assertion, revision_number, state, author,
		recorded_at, note, time_offset) VALUES (?, ?, ?, ?, ?, ?, ?)`, position, revision.RevisionNumber,
		string(revision.State), revision.Author, string(revision.RecordedAt), revision.Basis.Note, offset); err != nil {
		return fmt.Errorf("recording the revision %d: %w", revision.RevisionNumber, err)
	}
	for element, ref := range revision.Basis.RecordRefs {
		id, err := insertRecordRef(ctx, tx, ref)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO assertion_basis_ref (assertion, revision_number, element,
			record_ref) VALUES (?, ?, ?, ?)`, position, revision.RevisionNumber, element, id); err != nil {
			return fmt.Errorf("recording the basis of the revision %d: %w", revision.RevisionNumber, err)
		}
	}
	return nil
}

// readAssertions は所見を記録した順に読み、core の Validate を通して返す。
func (d *DB) readAssertions(ctx context.Context) ([]StoredAssertion, error) {
	refs, err := d.readRecordRefs(ctx)
	if err != nil {
		return nil, err
	}
	revisions, err := d.readRevisions(ctx, refs)
	if err != nil {
		return nil, err
	}
	proposalIds, err := d.readAdoptedProposalIds(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT position, id, ordinal, target_kind, target_node_id,
		target_edge_kind, target_edge_source_node_id, target_edge_target_node_id, target_record_ref,
		target_source_content_sha256, adds_relation FROM assertion ORDER BY position`)
	if err != nil {
		return nil, fmt.Errorf("reading the assertions: %w", err)
	}
	defer rows.Close()
	var assertions []StoredAssertion
	for rows.Next() {
		var position, ordinal int64
		var id, kind string
		var nodeId, edgeKind, edgeSource, edgeTarget, sourceContent sql.NullString
		var recordRef sql.NullInt64
		var addsRelation bool
		if err := rows.Scan(&position, &id, &ordinal, &kind, &nodeId, &edgeKind, &edgeSource, &edgeTarget,
			&recordRef, &sourceContent, &addsRelation); err != nil {
			return nil, fmt.Errorf("reading the assertions: %w", err)
		}
		target := core.AssertionTarget{
			Kind: core.AssertionTargetKind(kind), NodeId: nodeId.String, SourceContentSha256: sourceContent.String,
		}
		if edgeKind.Valid {
			target.Edge = &core.AssertionEdgeRef{
				Kind: core.EdgeKind(edgeKind.String), SourceNodeId: edgeSource.String, TargetNodeId: edgeTarget.String,
			}
		}
		if recordRef.Valid {
			ref, found := refs[recordRef.Int64]
			if !found {
				return nil, fmt.Errorf("reading the assertion %q: the target record reference is missing", id)
			}
			target.Record = &ref
		}
		assertion, err := assertionOf(id, target, addsRelation, proposalIds[position], revisions[position])
		if err != nil {
			return nil, err
		}
		assertions = append(assertions, StoredAssertion{Assertion: assertion, Ordinal: ordinal})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the assertions: %w", err)
	}
	return assertions, nil
}

// assertionOf は改訂の番号の昇順の並びから所見を組む。末尾の改訂が現在の改訂になり、残りが履歴になる。
func assertionOf(
	id string, target core.AssertionTarget, addsRelation bool, proposalId string, revisions []core.AssertionRevision,
) (core.Assertion, error) {
	if len(revisions) == 0 {
		return core.Assertion{}, fmt.Errorf("reading the assertion %q: it has no revision", id)
	}
	current := revisions[len(revisions)-1]
	assertion := core.Assertion{
		Id: id, Target: target, State: current.State, Author: current.Author, RecordedAt: current.RecordedAt,
		Basis: current.Basis, TimeOffset: current.TimeOffset, AddsRelation: addsRelation, ProposalId: proposalId,
		RevisionNumber: current.RevisionNumber, History: slices.Clone(revisions[:len(revisions)-1]),
	}
	if err := assertion.Validate(); err != nil {
		return core.Assertion{}, fmt.Errorf("reading the assertion %q: %w", id, err)
	}
	return assertion, nil
}

// readRevisions は所見の改訂を、所見の行の番号ごとに改訂の番号の昇順で読む。
func (d *DB) readRevisions(
	ctx context.Context, refs map[int64]core.AssertionRecordRef,
) (map[int64][]core.AssertionRevision, error) {
	basis, err := d.readBasisRefs(ctx, refs)
	if err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT assertion, revision_number, state, author, recorded_at, note,
		time_offset FROM assertion_revision ORDER BY assertion, revision_number`)
	if err != nil {
		return nil, fmt.Errorf("reading the assertion revisions: %w", err)
	}
	defer rows.Close()
	revisions := map[int64][]core.AssertionRevision{}
	for rows.Next() {
		var position int64
		var revision core.AssertionRevision
		var state, recordedAt string
		var offset sql.NullString
		if err := rows.Scan(&position, &revision.RevisionNumber, &state, &revision.Author, &recordedAt,
			&revision.Basis.Note, &offset); err != nil {
			return nil, fmt.Errorf("reading the assertion revisions: %w", err)
		}
		if offset.Valid {
			value := core.UtcOffset(offset.String)
			revision.TimeOffset = &value
		}
		revision.State, revision.RecordedAt = core.AssertionState(state), core.AssertionTime(recordedAt)
		revision.Basis.RecordRefs = basis[revisionKey{position, revision.RevisionNumber}]
		if revision.Basis.RecordRefs == nil {
			revision.Basis.RecordRefs = []core.AssertionRecordRef{}
		}
		revisions[position] = append(revisions[position], revision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the assertion revisions: %w", err)
	}
	return revisions, nil
}

type revisionKey struct{ assertion, revision int64 }

func (d *DB) readBasisRefs(
	ctx context.Context, refs map[int64]core.AssertionRecordRef,
) (map[revisionKey][]core.AssertionRecordRef, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT assertion, revision_number, record_ref FROM assertion_basis_ref
		ORDER BY assertion, revision_number, element`)
	if err != nil {
		return nil, fmt.Errorf("reading the assertion bases: %w", err)
	}
	defer rows.Close()
	basis := map[revisionKey][]core.AssertionRecordRef{}
	for rows.Next() {
		var key revisionKey
		var id int64
		if err := rows.Scan(&key.assertion, &key.revision, &id); err != nil {
			return nil, fmt.Errorf("reading the assertion bases: %w", err)
		}
		ref, found := refs[id]
		if !found {
			return nil, errors.New("reading the assertion bases: a basis record reference is missing")
		}
		basis[key] = append(basis[key], ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the assertion bases: %w", err)
	}
	return basis, nil
}
