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
	Resolve(ctx context.Context, q db.DBTX, p auth.Principal, space string) (spaces.Membership, error)
}

type inboxStager interface {
	Stage(ctx context.Context, tx db.DBTX, es []inbox.Entry) (inbox.Staged, error)
	Publish(st inbox.Staged) []inbox.Receipt
}

// Service runs blackboard operations; mutations run in one transaction each.
type Service struct {
	db     *sql.DB
	spaces membershipResolver
	ib     inboxStager
	now    func() time.Time
}

// NewService builds a blackboards Service.
func NewService(conn *sql.DB, r membershipResolver, ib inboxStager, now func() time.Time) *Service {
	return &Service{db: conn, spaces: r, ib: ib, now: now}
}
