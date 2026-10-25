// Package messaging implements direct messages and topics within spaces.
package messaging

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

const maxBody = 65536

type membershipResolver interface {
	Resolve(ctx context.Context, p auth.Principal, space string) (spaces.Membership, error)
	ResolveTx(ctx context.Context, q auth.DBTX, p auth.Principal, space string) (spaces.Membership, error)
}

type inboxWriter interface {
	Insert(ctx context.Context, tx auth.DBTX, es []inbox.Entry) ([]inbox.Entry, error)
	Publish(es []inbox.Entry)
}

// Service handles direct messages and topics.
type Service struct {
	db     *sql.DB
	spaces membershipResolver
	ib     inboxWriter
	now    func() time.Time
}

// NewService returns a messaging Service.
func NewService(db *sql.DB, r membershipResolver, ib inboxWriter, now func() time.Time) *Service {
	return &Service{db: db, spaces: r, ib: ib, now: now}
}

// txFunc does the work of one mutating op and returns the entries to deliver.
type txFunc func(tx *sql.Tx, m spaces.Membership) ([]inbox.Entry, error)

// inTx resolves p in space and runs fn in one transaction, delivering its entries after commit.
func (s *Service) inTx(ctx context.Context, p auth.Principal, space string, fn txFunc) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()
	m, err := s.spaces.ResolveTx(ctx, tx, p, space)
	if err != nil {
		return err
	}
	es, err := fn(tx, m)
	if err != nil {
		return err
	}
	inserted, err := s.ib.Insert(ctx, tx, es)
	if err != nil {
		return fmt.Errorf("inserting inbox entries: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	s.ib.Publish(inserted)
	return nil
}

func (s *Service) nowText() string { return s.now().UTC().Format(time.RFC3339Nano) }

func checkMessage(m spaces.Membership, body string, critical bool) error {
	if len(body) < 1 || len(body) > maxBody {
		return apperr.New(apperr.Invalid, "body must be 1..65536 bytes")
	}
	if critical && m.Role != spaces.RoleLeader {
		return apperr.New(apperr.Forbidden, "only leaders may send critical messages")
	}
	return nil
}

// fanOut copies proto once per recipient, stamping recipient, space and sender.
func fanOut(m spaces.Membership, recipients []int64, proto inbox.Entry) []inbox.Entry {
	es := make([]inbox.Entry, 0, len(recipients))
	for _, id := range recipients {
		e := proto
		e.AgentID, e.SpaceID, e.Space, e.From = id, m.SpaceID, m.SpaceName, m.AgentName
		es = append(es, e)
	}
	return es
}
