// Package messaging implements direct messages and topics within spaces.
package messaging

import (
	"context"
	"database/sql"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/domain"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

type membershipResolver interface {
	Resolve(ctx context.Context, p auth.Principal, space string) (spaces.Membership, error)
	ResolveTx(ctx context.Context, q db.DBTX, p auth.Principal, space string) (spaces.Membership, error)
	ParticipantID(ctx context.Context, q db.DBTX, spaceID int64, name string) (int64, bool, error)
	ParticipantIDs(ctx context.Context, q db.DBTX, spaceID int64) ([]int64, error)
}

type inboxDeliverer interface {
	Deliver(ctx context.Context, fn func(tx *sql.Tx) ([]inbox.Entry, error)) error
}

// Service handles direct messages and topics.
type Service struct {
	db     *sql.DB
	spaces membershipResolver
	ib     inboxDeliverer
	now    func() time.Time
}

// NewService returns a messaging Service.
func NewService(conn *sql.DB, r membershipResolver, ib inboxDeliverer, now func() time.Time) *Service {
	return &Service{db: conn, spaces: r, ib: ib, now: now}
}

// inSpace resolves p in space and runs fn in one delivering transaction.
func (s *Service) inSpace(ctx context.Context, p auth.Principal, space string,
	fn func(tx *sql.Tx, m spaces.Membership) ([]inbox.Entry, error)) error {
	return s.ib.Deliver(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
		m, err := s.spaces.ResolveTx(ctx, tx, p, space)
		if err != nil {
			return nil, err
		}
		return fn(tx, m)
	})
}

func checkMessage(m spaces.Membership, body string, critical bool) error {
	if err := domain.ValidBody(body); err != nil {
		return err
	}
	if critical && !m.Leads() {
		return apperr.New(apperr.Forbidden, "only leaders may send critical messages")
	}
	return nil
}

// without returns ids minus except, preserving order.
func without(ids []int64, except int64) []int64 {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id != except {
			out = append(out, id)
		}
	}
	return out
}
