package auth

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// Scope selects the tokens Revoke deletes: those bound to one space, or every token of the agent.
type Scope struct {
	spaceID int64
	all     bool
}

// InSpace is the scope of the tokens bound to spaceID; a leader's or admin's token is bound to none.
func InSpace(spaceID int64) Scope { return Scope{spaceID: spaceID} }

// Everywhere is the scope of every token of the agent.
func Everywhere() Scope { return Scope{all: true} }

// Revoke deletes agentID's tokens in scope through q and reports how many were removed.
func (s *Service) Revoke(ctx context.Context, q db.DBTX, agentID int64, scope Scope) (int64, error) {
	if scope.all {
		return deleteAgentTokens(ctx, q, agentID)
	}
	return deleteSpaceTokens(ctx, q, agentID, scope.spaceID)
}
