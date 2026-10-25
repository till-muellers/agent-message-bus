package spaces

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

// Resolver is the single authorization gate for space-scoped operations.
type Resolver struct{ db auth.DBTX }

// NewResolver returns a Resolver reading from db.
func NewResolver(db *sql.DB) *Resolver { return &Resolver{db: db} }

// Resolve returns p's membership in space or a forbidden/not-found error.
// NEVER call it while holding a transaction (the pool has one connection); use ResolveTx.
func (r *Resolver) Resolve(ctx context.Context, p auth.Principal, space string) (Membership, error) {
	return r.ResolveTx(ctx, r.db, p, space)
}

// ResolveTx is Resolve inside the caller's transaction q.
func (r *Resolver) ResolveTx(ctx context.Context, q auth.DBTX, p auth.Principal, space string) (Membership, error) {
	if p.Kind == auth.Admin {
		return Membership{}, apperr.New(apperr.Forbidden, "admin token cannot act in spaces; mint a leader with bus_mint_leader")
	}
	m := Membership{SpaceName: space, AgentID: p.AgentID, AgentName: p.AgentName}
	err := q.QueryRowContext(ctx, `SELECT id FROM spaces WHERE name = ?`, space).Scan(&m.SpaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return Membership{}, apperr.New(apperr.NotFound, "space %s not found", space)
	}
	if err != nil {
		return Membership{}, fmt.Errorf("looking up space %s: %w", space, err)
	}
	if p.Kind == auth.Member && p.SpaceID != m.SpaceID {
		return Membership{}, apperr.New(apperr.Forbidden, "token is not valid for space %s", space)
	}
	err = q.QueryRowContext(ctx, `SELECT role FROM participants WHERE space_id = ? AND agent_id = ?`,
		m.SpaceID, p.AgentID).Scan(&m.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return Membership{}, apperr.New(apperr.Forbidden, "not a participant of space %s", space)
	}
	if err != nil {
		return Membership{}, fmt.Errorf("looking up participant in %s: %w", space, err)
	}
	return m, nil
}
