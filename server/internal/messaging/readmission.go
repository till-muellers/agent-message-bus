package messaging

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// OnReadmission removes the re-invited member's topic subscriptions in the space.
// It implements spaces.ReadmissionHook.
func (s *Service) OnReadmission(ctx context.Context, tx db.DBTX, m spaces.Membership) error {
	return deleteSpaceSubscriptions(ctx, tx, m.SpaceID, m.AgentID)
}

var _ spaces.ReadmissionHook = (*Service)(nil)
