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
	Resolve(ctx context.Context, q db.DBTX, p auth.Principal, space string) (spaces.Membership, error)
	ParticipantID(ctx context.Context, q db.DBTX, spaceID int64, name string) (int64, bool, error)
	ParticipantIDs(ctx context.Context, q db.DBTX, spaceID int64) ([]int64, error)
}

type inboxStager interface {
	Stage(ctx context.Context, tx db.DBTX, es []inbox.Entry) (inbox.Staged, error)
	Publish(st inbox.Staged) []inbox.Receipt
}

// Service handles direct messages and topics.
type Service struct {
	db     *sql.DB
	spaces membershipResolver
	ib     inboxStager
	now    func() time.Time
}

// NewService returns a messaging Service.
func NewService(conn *sql.DB, r membershipResolver, ib inboxStager, now func() time.Time) *Service {
	return &Service{db: conn, spaces: r, ib: ib, now: now}
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
