package messaging

import (
	"context"
	"database/sql"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// SendDirect delivers body to participant to in space.
func (s *Service) SendDirect(ctx context.Context, p auth.Principal, space, to, body string, critical bool) error {
	return s.inSpace(ctx, p, space, func(tx *sql.Tx, m spaces.Membership) ([]inbox.Entry, error) {
		if err := checkMessage(m, body, critical); err != nil {
			return nil, err
		}
		if to == m.AgentName {
			return nil, apperr.New(apperr.Invalid, "cannot send a direct message to yourself")
		}
		id, found, err := s.spaces.ParticipantID(ctx, tx, m.SpaceID, to)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, apperr.New(apperr.NotFound, "participant %s not in space %s", to, space)
		}
		proto := inbox.Entry{Type: inbox.DirectMessage, Critical: critical, Body: body}
		return m.Notify([]int64{id}, proto), nil
	})
}
