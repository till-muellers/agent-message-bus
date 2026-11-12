package blackboards

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// OnReadmission removes the re-invited member's thread subscriptions in the space and keeps
// its boards. It implements spaces.ReadmissionHook.
func (s *Service) OnReadmission(ctx context.Context, tx db.DBTX, m spaces.Membership) error {
	return deleteSpaceSubscriptions(ctx, tx, m.SpaceID, m.AgentID)
}

var _ spaces.ReadmissionHook = (*Service)(nil)
