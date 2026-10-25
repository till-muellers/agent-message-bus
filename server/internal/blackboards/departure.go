package blackboards

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// OnDeparture implements spaces.DepartureHook: it drops the agent's thread subscriptions in
// the space and, for members, deletes their boards, notifying remaining subscribers.
func (s *Service) OnDeparture(ctx context.Context, tx auth.DBTX, m spaces.Membership) ([]inbox.Entry, error) {
	if err := deleteSpaceSubscriptions(ctx, tx, m.SpaceID, m.AgentID); err != nil {
		return nil, err
	}
	if m.Role != spaces.RoleMember {
		return nil, nil
	}
	boards, err := ownedBoards(ctx, tx, m.SpaceID, m.AgentID)
	if err != nil {
		return nil, err
	}
	var out []inbox.Entry
	for _, b := range boards {
		es, err := removeBoard(ctx, tx, m, b)
		if err != nil {
			return nil, err
		}
		out = append(out, es...)
	}
	return out, nil
}
