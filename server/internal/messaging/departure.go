package messaging

import (
	"context"
	"fmt"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// OnDeparture removes the departing agent's topic subscriptions in the space.
// It implements spaces.DepartureHook and returns no entries.
func (s *Service) OnDeparture(ctx context.Context, tx auth.DBTX, m spaces.Membership) ([]inbox.Entry, error) {
	_, err := tx.ExecContext(ctx, `DELETE FROM topic_subscriptions WHERE agent_id = ?
		AND topic_id IN (SELECT id FROM topics WHERE space_id = ?)`, m.AgentID, m.SpaceID)
	if err != nil {
		return nil, fmt.Errorf("deleting topic subscriptions of agent %d: %w", m.AgentID, err)
	}
	return nil, nil
}

var _ spaces.DepartureHook = (*Service)(nil)
