// Package blackboards implements blackboards, their threads, posts and thread subscriptions.
package blackboards

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

const (
	maxTitleChars = 200
	maxBodyBytes  = 65536
)

type membershipResolver interface {
	Resolve(ctx context.Context, p auth.Principal, space string) (spaces.Membership, error)
	ResolveTx(ctx context.Context, q db.DBTX, p auth.Principal, space string) (spaces.Membership, error)
}

type inboxWriter interface {
	Insert(ctx context.Context, tx db.DBTX, es []inbox.Entry) ([]inbox.Entry, error)
	Publish(es []inbox.Entry)
}

// Service runs blackboard operations, each in one transaction.
type Service struct {
	db     *sql.DB
	spaces membershipResolver
	ib     inboxWriter
	now    func() time.Time
}

// NewService builds a blackboards Service.
func NewService(db *sql.DB, r membershipResolver, ib inboxWriter, now func() time.Time) *Service {
	return &Service{db: db, spaces: r, ib: ib, now: now}
}

type txFunc func(tx *sql.Tx, m spaces.Membership) ([]inbox.Entry, error)

// inSpace resolves the caller in a transaction, runs fn, stores its entries, commits and publishes.
func (s *Service) inSpace(ctx context.Context, p auth.Principal, space string, fn txFunc) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
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
		return fmt.Errorf("committing: %w", err)
	}
	s.ib.Publish(inserted)
	return nil
}

func (s *Service) stamp() string { return s.now().UTC().Format(time.RFC3339Nano) }

func validTitle(title string) error {
	if n := utf8.RuneCountInString(title); n < 1 || n > maxTitleChars {
		return apperr.New(apperr.Invalid, "title must be 1..200 characters")
	}
	return nil
}

func validBody(body string) error {
	if len(body) < 1 || len(body) > maxBodyBytes {
		return apperr.New(apperr.Invalid, "body must be 1..65536 bytes")
	}
	return nil
}

// notify builds one entry of type t per recipient, sent by m in m's space.
func notify(m spaces.Membership, recipients []int64, t inbox.Type, tmpl inbox.Entry) []inbox.Entry {
	es := make([]inbox.Entry, 0, len(recipients))
	for _, id := range recipients {
		e := tmpl
		e.AgentID, e.SpaceID, e.Space, e.Type, e.From = id, m.SpaceID, m.SpaceName, t, m.AgentName
		es = append(es, e)
	}
	return es
}
