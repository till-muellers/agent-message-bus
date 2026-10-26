package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// lookupToken resolves a token hash to its principal; found is false when unknown.
func lookupToken(ctx context.Context, q db.DBTX, hash string) (p Principal, found bool, err error) {
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
func findAgent(ctx context.Context, q db.DBTX, name string) (id int64, kind Kind, found bool, err error) {
	err = q.QueryRowContext(ctx, `SELECT id, kind FROM agents WHERE name = ?`, name).Scan(&id, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("finding agent %s: %w", name, err)
	}
	return id, kind, true, nil
}

func insertAgent(ctx context.Context, q db.DBTX, name string, kind Kind, createdAt string) (int64, error) {
	id, err := db.InsertID(ctx, q, `INSERT INTO agents (name, kind, created_at) VALUES (?, ?, ?)`,
		name, string(kind), createdAt)
	if err != nil {
		return 0, fmt.Errorf("inserting agent %s: %w", name, err)
	}
	return id, nil
}

// insertToken stores hash for agentID; spaceID 0 stores a leader token (NULL space).
func insertToken(ctx context.Context, q db.DBTX, hash string, agentID, spaceID int64, createdAt string) error {
	space := sql.NullInt64{Int64: spaceID, Valid: spaceID != 0}
	_, err := q.ExecContext(ctx, `INSERT INTO tokens (hash, agent_id, space_id, created_at) VALUES (?, ?, ?, ?)`,
		hash, agentID, space, createdAt)
	if err != nil {
		return fmt.Errorf("inserting token: %w", err)
	}
	return nil
}

// deleteSpaceTokens removes every token of agentID bound to spaceID.
func deleteSpaceTokens(ctx context.Context, q db.DBTX, agentID, spaceID int64) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM tokens WHERE agent_id = ? AND space_id = ?`, agentID, spaceID); err != nil {
		return fmt.Errorf("deleting space tokens: %w", err)
	}
	return nil
}

// listAdmins reads every admin agent by name with its token count and joined spaces.
func listAdmins(ctx context.Context, q db.DBTX) ([]AdminInfo, error) {
	rows, err := q.QueryContext(ctx, `SELECT a.name,
		(SELECT COUNT(*) FROM tokens t WHERE t.agent_id = a.id),
		COALESCE((SELECT group_concat(name, char(31)) FROM (SELECT s.name FROM participants p
			JOIN spaces s ON s.id = p.space_id WHERE p.agent_id = a.id ORDER BY s.name)), '')
		FROM agents a WHERE a.kind = 'admin' ORDER BY a.name`)
	if err != nil {
		return nil, fmt.Errorf("listing admins: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []AdminInfo{}
	for rows.Next() {
		var a AdminInfo
		var spaces string
		if err := rows.Scan(&a.Name, &a.Tokens, &spaces); err != nil {
			return nil, fmt.Errorf("scanning admin: %w", err)
		}
		a.Spaces = splitNames(spaces)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating admins: %w", err)
	}
	return out, nil
}

// splitNames splits the unit-separator joined list s; empty s yields an empty slice.
func splitNames(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "\x1f")
}
