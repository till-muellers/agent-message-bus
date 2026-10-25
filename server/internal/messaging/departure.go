package messaging

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// OnDeparture removes the departing agent's topic subscriptions in the space.
// It implements spaces.DepartureHook and returns no entries.
func (s *Service) OnDeparture(ctx context.Context, tx db.DBTX, m spaces.Membership) ([]inbox.Entry, error) {
	return nil, deleteSpaceSubscriptions(ctx, tx, m.SpaceID, m.AgentID)
}

var _ spaces.DepartureHook = (*Service)(nil)
