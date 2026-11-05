package messaging

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// SendDirect delivers body to participant to in space and returns its receipt.
func (s *Service) SendDirect(ctx context.Context, p auth.Principal, space, to, body string, critical bool) ([]inbox.Receipt, error) {
	return inbox.InTx(ctx, s.db, s.ib, func(tx db.DBTX) ([]inbox.Entry, error) {
		m, err := s.spaces.Resolve(ctx, tx, p, space)
		if err != nil {
			return nil, err
		}
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
