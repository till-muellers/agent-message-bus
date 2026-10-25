package invites

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// row is one stored invite; task is NULL when the invite has no first prompt.
type row struct {
	hash      string
	spaceID   int64
	name      string
	task      sql.NullString
	expiresAt int64 // unix seconds
}

// insertInvite stores r under the hash of its code.
func insertInvite(ctx context.Context, q db.DBTX, r row) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO invites (hash, space_id, name, task, expires_at) VALUES (?, ?, ?, ?, ?)`,
		r.hash, r.spaceID, r.name, r.task, r.expiresAt)
	if err != nil {
		return fmt.Errorf("inserting invite for %s: %w", r.name, err)
	}
	return nil
}

// takeInvite reads and deletes the invite stored under hash.
func takeInvite(ctx context.Context, q db.DBTX, hash string) (row, bool, error) {
	r := row{hash: hash}
	err := q.QueryRowContext(ctx, `SELECT space_id, name, task, expires_at FROM invites WHERE hash = ?`, hash).
		Scan(&r.spaceID, &r.name, &r.task, &r.expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return row{}, false, nil
	}
	if err != nil {
		return row{}, false, fmt.Errorf("looking up invite: %w", err)
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM invites WHERE hash = ?`, hash); err != nil {
		return row{}, false, fmt.Errorf("deleting invite: %w", err)
	}
	return r, true, nil
}

// deleteExpired removes every invite that expired at or before now (unix seconds).
func deleteExpired(ctx context.Context, q db.DBTX, now int64) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM invites WHERE expires_at <= ?`, now); err != nil {
		return fmt.Errorf("deleting expired invites: %w", err)
	}
	return nil
}

// deleteForName removes every invite for name in spaceID and reports whether any existed.
func deleteForName(ctx context.Context, q db.DBTX, spaceID int64, name string) (bool, error) {
	res, err := q.ExecContext(ctx, `DELETE FROM invites WHERE space_id = ? AND name = ?`, spaceID, name)
	if err != nil {
		return false, fmt.Errorf("deleting invites for %s: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("counting deleted invites for %s: %w", name, err)
	}
	return n > 0, nil
}
