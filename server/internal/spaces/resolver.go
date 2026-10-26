package spaces

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// Resolver is the single authorization gate for space-scoped operations.
type Resolver struct{ db db.DBTX }

// NewResolver returns a Resolver reading from db.
func NewResolver(conn *sql.DB) *Resolver { return &Resolver{db: conn} }

// requireAgent rejects the admin principal where admins have no rights: listing and deleting spaces.
func requireAgent(p auth.Principal) error {
	if p.Kind == auth.Admin {
		return apperr.New(apperr.Forbidden, "admin token cannot act in spaces; mint a leader with bus_mint_leader")
	}
	return nil
}

// requireLeaderPrincipal rejects every principal that is not a leader agent.
func requireLeaderPrincipal(p auth.Principal, action string) error {
	if p.Kind != auth.Leader {
		return apperr.New(apperr.Forbidden, "only leaders may %s", action)
	}
	return nil
}

// ParticipantID returns the agent id of participant name in spaceID; found is false otherwise.
func (r *Resolver) ParticipantID(ctx context.Context, q db.DBTX, spaceID int64, name string) (id int64, found bool, err error) {
	id, _, found, err = participantByName(ctx, q, spaceID, name)
	return id, found, err
}

// ParticipantIDs returns the agent ids of every participant of spaceID, ascending.
func (r *Resolver) ParticipantIDs(ctx context.Context, q db.DBTX, spaceID int64) ([]int64, error) {
	return participantIDs(ctx, q, spaceID)
}

// Resolve returns p's membership in space or a forbidden/not-found error.
// NEVER call it while holding a transaction (the pool has one connection); use ResolveTx.
func (r *Resolver) Resolve(ctx context.Context, p auth.Principal, space string) (Membership, error) {
	return r.ResolveTx(ctx, r.db, p, space)
}

// ResolveTx is Resolve inside the caller's transaction q. An admin resolves in every
// space with Admin set; its Role is its participant role, or "" when it has not joined.
func (r *Resolver) ResolveTx(ctx context.Context, q db.DBTX, p auth.Principal, space string) (Membership, error) {
	id, found, err := spaceIDByName(ctx, q, space)
	if err != nil {
		return Membership{}, err
	}
	if !found {
		return Membership{}, apperr.New(apperr.NotFound, "space %s not found", space)
	}
	if p.Kind == auth.Member && p.SpaceID != id {
		return Membership{}, apperr.New(apperr.Forbidden, "token is not valid for space %s", space)
	}
	role, found, err := participantRole(ctx, q, id, p.AgentID)
	if err != nil {
		return Membership{}, fmt.Errorf("resolving %s: %w", space, err)
	}
	m := Membership{SpaceID: id, SpaceName: space, AgentID: p.AgentID, AgentName: p.AgentName, Role: role,
		Admin: p.Kind == auth.Admin}
	if !found && !m.Admin {
		return Membership{}, apperr.New(apperr.Forbidden, "not a participant of space %s", space)
	}
	return m, nil
}
