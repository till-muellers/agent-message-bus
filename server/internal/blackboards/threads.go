package blackboards

import (
	"context"
	"database/sql"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/domain"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// ThreadInfo summarizes a thread on a blackboard.
type ThreadInfo struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	CreatedBy  string `json:"createdBy"`
	Posts      int    `json:"posts"`
	LastPostAt string `json:"lastPostAt"`
}

// Thread is a thread with all its posts.
type Thread struct {
	ID         int64  `json:"id"`
	Blackboard string `json:"blackboard"`
	Title      string `json:"title"`
	CreatedBy  string `json:"createdBy"`
	Subscribed bool   `json:"subscribed"`
	Posts      []Post `json:"posts"`
}

// Post is one post in a thread.
type Post struct {
	ID        int64  `json:"id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
}

// RecentThread is a thread of any board in a space with the caller's subscription state.
type RecentThread struct {
	ID         int64  `json:"id"`
	Blackboard string `json:"blackboard"`
	Title      string `json:"title"`
	CreatedBy  string `json:"createdBy"`
	Posts      int    `json:"posts"`
	LastPostAt string `json:"lastPostAt"`
	Subscribed bool   `json:"subscribed"`
}

// ListThreads returns a board's threads ordered by latest post, newest first.
func (s *Service) ListThreads(ctx context.Context, p auth.Principal, space, name string) ([]ThreadInfo, error) {
	m, err := s.spaces.Resolve(ctx, s.db, p, space)
	if err != nil {
		return nil, err
	}
	b, err := mustFindBoard(ctx, s.db, m.SpaceID, name)
	if err != nil {
		return nil, err
	}
	return listThreads(ctx, s.db, b.id)
}

// RecentThreadsFor returns at most limit threads across all boards of m's space, latest
// post first, each with whether m's agent is subscribed.
func (s *Service) RecentThreadsFor(ctx context.Context, m spaces.Membership, limit int) ([]RecentThread, error) {
	return recentThreads(ctx, s.db, m.SpaceID, m.AgentID, limit)
}

// CreateThread inserts a thread with its first post and subscribes the creator.
func (s *Service) CreateThread(ctx context.Context, p auth.Principal, space, name, title, body string) (int64, error) {
	if err := domain.ValidTitle(title); err != nil {
		return 0, err
	}
	if err := domain.ValidBody(body); err != nil {
		return 0, err
	}
	var id int64
	err := db.InTx(ctx, s.db, func(tx *sql.Tx) error {
		m, err := s.spaces.Resolve(ctx, tx, p, space)
		if err != nil {
			return err
		}
		b, err := mustFindBoard(ctx, tx, m.SpaceID, name)
		if err != nil {
			return err
		}
		at := db.Timestamp(s.now())
		if id, err = insertThread(ctx, tx, b.id, title, m.AgentID, at); err != nil {
			return err
		}
		if _, err = insertPost(ctx, tx, id, m.AgentID, body, at); err != nil || !m.Joined() {
			return err // only participants are subscribed; an admin that has not joined is not
		}
		return subscribe(ctx, tx, id, m.AgentID)
	})
	return id, err
}

// ReadThread returns a thread with its posts ordered by id.
func (s *Service) ReadThread(ctx context.Context, p auth.Principal, space string, id int64) (Thread, error) {
	m, err := s.spaces.Resolve(ctx, s.db, p, space)
	if err != nil {
		return Thread{}, err
	}
	t, err := mustFindThread(ctx, s.db, m.SpaceID, id)
	if err != nil {
		return Thread{}, err
	}
	out := Thread{ID: t.id, Blackboard: t.board, Title: t.title, CreatedBy: t.createdBy}
	if out.Subscribed, err = isSubscribed(ctx, s.db, id, m.AgentID); err != nil {
		return Thread{}, err
	}
	if out.Posts, err = listPosts(ctx, s.db, id); err != nil {
		return Thread{}, err
	}
	return out, nil
}

// RemoveThread deletes a thread (creator or leader), notifying subscribers except the caller.
func (s *Service) RemoveThread(ctx context.Context, p auth.Principal, space string, id int64) error {
	_, err := inbox.InTx(ctx, s.db, s.ib, func(tx db.DBTX) ([]inbox.Entry, error) {
		m, err := s.spaces.Resolve(ctx, tx, p, space)
		if err != nil {
			return nil, err
		}
		t, err := mustFindThread(ctx, tx, m.SpaceID, id)
		if err != nil {
			return nil, err
		}
		if t.createdByID != m.AgentID && !m.Leads() {
			return nil, apperr.New(apperr.Forbidden, "only the creator or a leader may remove thread %d", id)
		}
		ids, err := threadSubscribers(ctx, tx, id, m.AgentID)
		if err != nil {
			return nil, err
		}
		if err := deleteThread(ctx, tx, id); err != nil {
			return nil, err
		}
		proto := inbox.Entry{Type: inbox.ThreadRemoved, Blackboard: t.board, ThreadID: id, ThreadTitle: t.title}
		return m.Notify(ids, proto), nil
	})
	return err
}

func mustFindThread(ctx context.Context, q db.DBTX, spaceID, id int64) (thread, error) {
	t, found, err := findThread(ctx, q, spaceID, id)
	if err != nil {
		return thread{}, err
	}
	if !found {
		return thread{}, apperr.New(apperr.NotFound, "thread %d not found", id)
	}
	return t, nil
}
