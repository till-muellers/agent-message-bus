package spaces

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

type spaceRow struct {
	id   int64
	name string
}

// listSpaces returns all spaces (spaceID 0) or only spaceID, ordered by name.
func listSpaces(ctx context.Context, q db.DBTX, spaceID int64) ([]spaceRow, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, name FROM spaces WHERE ? = 0 OR id = ? ORDER BY name`, spaceID, spaceID)
	if err != nil {
		return nil, fmt.Errorf("listing spaces: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []spaceRow
	for rows.Next() {
		var r spaceRow
		if err := rows.Scan(&r.id, &r.name); err != nil {
			return nil, fmt.Errorf("scanning space: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating spaces: %w", err)
	}
	return out, nil
}

// spaceExists reports whether a space called name exists.
func spaceExists(ctx context.Context, q db.DBTX, name string) (bool, error) {
	var n int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM spaces WHERE name = ?`, name).Scan(&n); err != nil {
		return false, fmt.Errorf("checking space %s: %w", name, err)
	}
	return n > 0, nil
}

// insertSpace creates a space and returns its id.
func insertSpace(ctx context.Context, q db.DBTX, name string, creator int64, ts string) (int64, error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO spaces (name, created_by, created_at) VALUES (?, ?, ?)`, name, creator, ts)
	if err != nil {
		return 0, fmt.Errorf("inserting space %s: %w", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("reading space id: %w", err)
	}
	return id, nil
}

func insertAllTopic(ctx context.Context, q db.DBTX, spaceID, creator int64, ts string) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO topics (space_id, name, created_by, created_at) VALUES (?, 'all', ?, ?)`, spaceID, creator, ts)
	if err != nil {
		return fmt.Errorf("inserting all topic: %w", err)
	}
	return nil
}

func insertParticipant(ctx context.Context, q db.DBTX, spaceID, agentID int64, role Role, ts string) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO participants (space_id, agent_id, role, joined_at) VALUES (?, ?, ?, ?)`,
		spaceID, agentID, role, ts)
	if err != nil {
		return fmt.Errorf("inserting participant: %w", err)
	}
	return nil
}

// participantRole returns the agent's role in the space, or found=false.
func participantRole(ctx context.Context, q db.DBTX, spaceID, agentID int64) (Role, bool, error) {
	var role Role
	err := q.QueryRowContext(ctx, `SELECT role FROM participants WHERE space_id = ? AND agent_id = ?`,
		spaceID, agentID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("looking up participant: %w", err)
	}
	return role, true, nil
}

// participantByName returns the agent id and role of participant name.
func participantByName(ctx context.Context, q db.DBTX, spaceID int64, name string) (int64, Role, bool, error) {
	var id int64
	var role Role
	err := q.QueryRowContext(ctx, `SELECT a.id, p.role FROM participants p JOIN agents a ON a.id = p.agent_id
		WHERE p.space_id = ? AND a.name = ?`, spaceID, name).Scan(&id, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("looking up participant %s: %w", name, err)
	}
	return id, role, true, nil
}

func countOtherLeaders(ctx context.Context, q db.DBTX, spaceID, agentID int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM participants
		WHERE space_id = ? AND role = 'leader' AND agent_id <> ?`, spaceID, agentID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting leaders: %w", err)
	}
	return n, nil
}

func deleteSpace(ctx context.Context, q db.DBTX, spaceID int64) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM spaces WHERE id = ?`, spaceID); err != nil {
		return fmt.Errorf("deleting space: %w", err)
	}
	return nil
}

// deleteParticipantData removes inbox rows, space tokens and the participant row.
func deleteParticipantData(ctx context.Context, q db.DBTX, spaceID, agentID int64) error {
	stmts := []string{
		`DELETE FROM inbox WHERE agent_id = ? AND space_id = ?`,
		`DELETE FROM tokens WHERE agent_id = ? AND space_id = ?`,
		`DELETE FROM participants WHERE agent_id = ? AND space_id = ?`,
	}
	for _, st := range stmts {
		if _, err := q.ExecContext(ctx, st, agentID, spaceID); err != nil {
			return fmt.Errorf("removing participant data: %w", err)
		}
	}
	return nil
}

// listParticipants returns participants ordered leaders first, then by name.
func listParticipants(ctx context.Context, q db.DBTX, spaceID int64) ([]Participant, error) {
	rows, err := q.QueryContext(ctx, `SELECT a.name, p.role FROM participants p JOIN agents a ON a.id = p.agent_id
		WHERE p.space_id = ? ORDER BY CASE p.role WHEN 'leader' THEN 0 ELSE 1 END, a.name`, spaceID)
	if err != nil {
		return nil, fmt.Errorf("listing participants: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []Participant{}
	for rows.Next() {
		var p Participant
		if err := rows.Scan(&p.Name, &p.Role); err != nil {
			return nil, fmt.Errorf("scanning participant: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating participants: %w", err)
	}
	return out, nil
}

// agentSpaces lists the spaces agentID participates in, ordered by name.
func agentSpaces(ctx context.Context, q db.DBTX, agentID int64) ([]SpaceRole, error) {
	rows, err := q.QueryContext(ctx, `SELECT s.name, p.role FROM participants p JOIN spaces s ON s.id = p.space_id
		WHERE p.agent_id = ? ORDER BY s.name`, agentID)
	if err != nil {
		return nil, fmt.Errorf("listing agent spaces: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []SpaceRole{}
	for rows.Next() {
		var r SpaceRole
		if err := rows.Scan(&r.Name, &r.Role); err != nil {
			return nil, fmt.Errorf("scanning agent space: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating agent spaces: %w", err)
	}
	return out, nil
}
