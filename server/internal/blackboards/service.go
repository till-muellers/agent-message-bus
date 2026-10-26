// Package blackboards implements blackboards, their threads, posts and thread subscriptions.
package blackboards

import (
	"context"
	"database/sql"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

type membershipResolver interface {
	Resolve(ctx context.Context, p auth.Principal, space string) (spaces.Membership, error)
	ResolveTx(ctx context.Context, q db.DBTX, p auth.Principal, space string) (spaces.Membership, error)
}

type inboxDeliverer interface {
	DeliverReceipts(ctx context.Context, fn func(tx *sql.Tx) ([]inbox.Entry, error)) ([]inbox.Receipt, error)
}

// Service runs blackboard operations; mutations run in one transaction each.
type Service struct {
	db     *sql.DB
	spaces membershipResolver
	ib     inboxDeliverer
	now    func() time.Time
}

// NewService builds a blackboards Service.
func NewService(conn *sql.DB, r membershipResolver, ib inboxDeliverer, now func() time.Time) *Service {
	return &Service{db: conn, spaces: r, ib: ib, now: now}
}

// inSpace resolves the caller inside a delivery transaction and runs fn there.
func (s *Service) inSpace(ctx context.Context, p auth.Principal, space string,
	fn func(tx *sql.Tx, m spaces.Membership) ([]inbox.Entry, error)) error {
	_, err := s.inSpaceReceipts(ctx, p, space, fn)
	return err
}

// inSpaceReceipts is inSpace returning a receipt per recipient (inbox.DeliverReceipts).
func (s *Service) inSpaceReceipts(ctx context.Context, p auth.Principal, space string,
	fn func(tx *sql.Tx, m spaces.Membership) ([]inbox.Entry, error)) ([]inbox.Receipt, error) {
	return s.ib.DeliverReceipts(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
		m, err := s.spaces.ResolveTx(ctx, tx, p, space)
		if err != nil {
			return nil, err
		}
		return fn(tx, m)
	})
}
