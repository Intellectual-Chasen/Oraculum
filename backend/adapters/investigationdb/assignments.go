package investigationdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// InsertAssignment は分析者が与えた端末の割当 1 件を、1 つの transaction で書く。
func (d *DB) InsertAssignment(ctx context.Context, assignment core.TerminalAssignment) error {
	err := d.transaction(ctx, func(tx *sql.Tx) error {
		from, err := insertTimestamp(ctx, tx, assignment.AssignmentValidRange.From)
		if err != nil {
			return err
		}
		to, err := insertTimestamp(ctx, tx, assignment.AssignmentValidRange.To)
		if err != nil {
			return err
		}
		// ホスト名は制御文字と空白を持たない (core.TerminalAssignment.Validate)。改行で区切って 1 列に置く。
		result, err := tx.ExecContext(ctx, `INSERT INTO terminal_assignment (client_ip, terminal_id,
			terminal_hostname, terminal_hostnames, source_id, source_content_sha256, valid_from, valid_to, origin,
			derivation, author, applies_to_source_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			assignment.ClientIp, assignment.TerminalId, assignment.TerminalHostname,
			strings.Join(assignment.TerminalHostnames, "\n"), assignment.SourceId, assignment.SourceContentSha256,
			from, to, string(assignment.Origin), assignment.Derivation, assignment.Author,
			assignment.AppliesToSourceId)
		if err != nil {
			return fmt.Errorf("recording the assignment: %w", err)
		}
		position, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("recording the assignment: %w", err)
		}
		for element, ref := range assignment.BasisRecordRefs {
			id, err := insertRecordRef(ctx, tx, ref)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO terminal_assignment_basis_ref (assignment, element,
				record_ref) VALUES (?, ?, ?)`, position, element, id); err != nil {
				return fmt.Errorf("recording the basis of the assignment: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		// 分析者が書いた文字列を error に載せない。呼び出し元が log へ出す。
		return fmt.Errorf("recording the terminal assignment: %w", err)
	}
	return nil
}

// readAssignments は割当を記録した順に読み、core の Validate を通して返す。
func (d *DB) readAssignments(ctx context.Context) ([]core.TerminalAssignment, error) {
	refs, err := d.readRecordRefs(ctx)
	if err != nil {
		return nil, err
	}
	timestamps, err := d.readTimestamps(ctx)
	if err != nil {
		return nil, err
	}
	basis, err := d.readAssignmentBasis(ctx, refs)
	if err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT position, client_ip, terminal_id, terminal_hostname,
		terminal_hostnames, source_id, source_content_sha256, valid_from, valid_to, origin, derivation, author,
		applies_to_source_id FROM terminal_assignment ORDER BY position`)
	if err != nil {
		return nil, fmt.Errorf("reading the terminal assignments: %w", err)
	}
	defer rows.Close()
	var assignments []core.TerminalAssignment
	for rows.Next() {
		var position, from, to int64
		var assignment core.TerminalAssignment
		var origin, hostnames string
		if err := rows.Scan(&position, &assignment.ClientIp, &assignment.TerminalId, &assignment.TerminalHostname,
			&hostnames, &assignment.SourceId, &assignment.SourceContentSha256, &from, &to, &origin, &assignment.Derivation,
			&assignment.Author, &assignment.AppliesToSourceId); err != nil {
			return nil, fmt.Errorf("reading the terminal assignments: %w", err)
		}
		fromValue, fromFound := timestamps[from]
		toValue, toFound := timestamps[to]
		if !fromFound || !toFound {
			return nil, errors.New("reading the terminal assignments: a bound of the valid range is missing")
		}
		assignment.AssignmentValidRange = core.TimeRange{From: fromValue, To: toValue}
		assignment.Origin = core.TerminalAssignmentOrigin(origin)
		if hostnames != "" {
			assignment.TerminalHostnames = strings.Split(hostnames, "\n")
		}
		assignment.BasisRecordRefs = basis[position]
		if err := assignment.Validate(); err != nil {
			return nil, fmt.Errorf("reading the terminal assignment %d: %w", position, err)
		}
		assignments = append(assignments, assignment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the terminal assignments: %w", err)
	}
	return assignments, nil
}

func (d *DB) readAssignmentBasis(
	ctx context.Context, refs map[int64]core.AssertionRecordRef,
) (map[int64][]core.AssertionRecordRef, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT assignment, record_ref FROM terminal_assignment_basis_ref
		ORDER BY assignment, element`)
	if err != nil {
		return nil, fmt.Errorf("reading the terminal assignment bases: %w", err)
	}
	defer rows.Close()
	basis := map[int64][]core.AssertionRecordRef{}
	for rows.Next() {
		var position, id int64
		if err := rows.Scan(&position, &id); err != nil {
			return nil, fmt.Errorf("reading the terminal assignment bases: %w", err)
		}
		ref, found := refs[id]
		if !found {
			return nil, errors.New("reading the terminal assignment bases: a basis record reference is missing")
		}
		basis[position] = append(basis[position], ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the terminal assignment bases: %w", err)
	}
	return basis, nil
}
