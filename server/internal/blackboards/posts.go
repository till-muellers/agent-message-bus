package blackboards

import (
	"context"
	"database/sql"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// AddPost appends a post and notifies the thread's subscribers except the author.
func (s *Service) AddPost(ctx context.Context, p auth.Principal, space string, id int64, body string) (int64, error) {
	if err := validBody(body); err != nil {
		return 0, err
	}
	var postID int64
	err := s.inSpace(ctx, p, space, func(tx *sql.Tx, m spaces.Membership) ([]inbox.Entry, error) {
		t, err := mustFindThread(ctx, tx, m.SpaceID, id)
		if err != nil {
			return nil, err
		}
		if postID, err = insertPost(ctx, tx, id, m.AgentID, body, s.stamp()); err != nil {
			return nil, err
		}
		ids, err := threadSubscribers(ctx, tx, id, m.AgentID)
		if err != nil {
			return nil, err
		}
		tmpl := inbox.Entry{Blackboard: t.board, ThreadID: id, ThreadTitle: t.title, Body: body}
		return notify(m, ids, inbox.ThreadPost, tmpl), nil
	})
	return postID, err
}

// Subscribe subscribes the caller to a thread; idempotent.
func (s *Service) Subscribe(ctx context.Context, p auth.Principal, space string, id int64) error {
	return s.inSpace(ctx, p, space, func(tx *sql.Tx, m spaces.Membership) ([]inbox.Entry, error) {
		if _, err := mustFindThread(ctx, tx, m.SpaceID, id); err != nil {
			return nil, err
		}
		return nil, subscribe(ctx, tx, id, m.AgentID)
	})
}

// Unsubscribe removes the caller's subscription to a thread; idempotent.
func (s *Service) Unsubscribe(ctx context.Context, p auth.Principal, space string, id int64) error {
	return s.inSpace(ctx, p, space, func(tx *sql.Tx, m spaces.Membership) ([]inbox.Entry, error) {
		if _, err := mustFindThread(ctx, tx, m.SpaceID, id); err != nil {
			return nil, err
		}
		return nil, unsubscribe(ctx, tx, id, m.AgentID)
	})
}
