package spaces

import (
	"context"
	"fmt"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// Resolver is the single authorization gate for space-scoped operations. It holds no
// connection: every method reads through the q its caller passes, the caller's
// transaction or the database.
type Resolver struct{}

// NewResolver returns a Resolver.
func NewResolver() *Resolver { return &Resolver{} }

// requireLeaderPrincipal rejects every principal that is neither a leader nor an admin agent.
func requireLeaderPrincipal(p auth.Principal, action string) error {
	if p.Kind != auth.Leader && p.Kind != auth.Admin {
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

// Resolve returns p's membership in space, read through q, or a forbidden/not-found
// error. An admin resolves in every space with Admin set; its Role is its participant
// role, or "" when it has not joined.
func (r *Resolver) Resolve(ctx context.Context, q db.DBTX, p auth.Principal, space string) (Membership, error) {
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
