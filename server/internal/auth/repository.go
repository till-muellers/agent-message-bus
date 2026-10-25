package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// lookupToken resolves a token hash to its principal; found is false when unknown.
func lookupToken(ctx context.Context, q DBTX, hash string) (p Principal, found bool, err error) {
	var space sql.NullInt64
	err = q.QueryRowContext(ctx, `SELECT a.id, a.name, a.kind, t.space_id
		FROM tokens t JOIN agents a ON a.id = t.agent_id WHERE t.hash = ?`, hash).
		Scan(&p.AgentID, &p.AgentName, &p.Kind, &space)
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, false, nil
	}
	if err != nil {
		return Principal{}, false, fmt.Errorf("looking up token: %w", err)
	}
	p.SpaceID = space.Int64
	return p, true, nil
}

// findAgent returns the id and kind of the named agent; found is false when missing.
func findAgent(ctx context.Context, q DBTX, name string) (id int64, kind Kind, found bool, err error) {
	err = q.QueryRowContext(ctx, `SELECT id, kind FROM agents WHERE name = ?`, name).Scan(&id, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("finding agent %s: %w", name, err)
	}
	return id, kind, true, nil
}

func insertAgent(ctx context.Context, q DBTX, name string, kind Kind, createdAt string) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO agents (name, kind, created_at) VALUES (?, ?, ?)`,
		name, string(kind), createdAt)
	if err != nil {
		return 0, fmt.Errorf("inserting agent %s: %w", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("reading agent id: %w", err)
	}
	return id, nil
}

// insertToken stores hash for agentID; spaceID 0 stores a leader token (NULL space).
func insertToken(ctx context.Context, q DBTX, hash string, agentID, spaceID int64, createdAt string) error {
	space := sql.NullInt64{Int64: spaceID, Valid: spaceID != 0}
	_, err := q.ExecContext(ctx, `INSERT INTO tokens (hash, agent_id, space_id, created_at) VALUES (?, ?, ?, ?)`,
		hash, agentID, space, createdAt)
	if err != nil {
		return fmt.Errorf("inserting token: %w", err)
	}
	return nil
}
