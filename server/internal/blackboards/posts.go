package blackboards

import (
	"context"
	"database/sql"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/domain"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// AddPost appends a post, notifies the thread's subscribers except the author and returns
// the post id with a receipt per notified subscriber, sorted by name.
func (s *Service) AddPost(ctx context.Context, p auth.Principal, space string, id int64, body string) (int64, []inbox.Receipt, error) {
	if err := domain.ValidBody(body); err != nil {
		return 0, nil, err
	}
	var postID int64
	rs, err := inbox.InTx(ctx, s.db, s.ib, func(tx db.DBTX) ([]inbox.Entry, error) {
		m, err := s.spaces.Resolve(ctx, tx, p, space)
		if err != nil {
			return nil, err
		}
		t, err := mustFindThread(ctx, tx, m.SpaceID, id)
		if err != nil {
			return nil, err
		}
		if postID, err = insertPost(ctx, tx, id, m.AgentID, body, db.Timestamp(s.now())); err != nil {
			return nil, err
		}
		ids, err := threadSubscribers(ctx, tx, id, m.AgentID)
		if err != nil {
			return nil, err
		}
		tmpl := inbox.Entry{Type: inbox.ThreadPost, Blackboard: t.board, ThreadID: id, ThreadTitle: t.title, Body: body}
		return m.Notify(ids, tmpl), nil
	})
	return postID, rs, err
}

// Subscribe subscribes the caller to a thread; idempotent.
func (s *Service) Subscribe(ctx context.Context, p auth.Principal, space string, id int64) error {
	return s.changeSubscription(ctx, p, space, id, subscribe)
}

// Unsubscribe removes the caller's subscription to a thread; idempotent.
func (s *Service) Unsubscribe(ctx context.Context, p auth.Principal, space string, id int64) error {
	return s.changeSubscription(ctx, p, space, id, unsubscribe)
}

type subscriptionOp func(ctx context.Context, q db.DBTX, threadID, agentID int64) error

// changeSubscription applies op to the joined caller's subscription of thread id.
func (s *Service) changeSubscription(ctx context.Context, p auth.Principal, space string, id int64, op subscriptionOp) error {
	return db.InTx(ctx, s.db, func(tx *sql.Tx) error {
		m, err := s.spaces.Resolve(ctx, tx, p, space)
		if err != nil {
			return err
		}
		if err := m.RequireJoined(); err != nil {
			return err
		}
		if _, err := mustFindThread(ctx, tx, m.SpaceID, id); err != nil {
			return err
		}
		return op(ctx, tx, id, m.AgentID)
	})
}
