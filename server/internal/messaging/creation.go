package messaging

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// OnSpaceCreated opens the all topic of the new space, created by m's agent.
// It implements spaces.CreationHook.
func (s *Service) OnSpaceCreated(ctx context.Context, tx db.DBTX, m spaces.Membership) error {
	_, err := insertTopic(ctx, tx, m.SpaceID, allTopic, m.AgentID, db.Timestamp(s.now()))
	return err
}

var _ spaces.CreationHook = (*Service)(nil)
