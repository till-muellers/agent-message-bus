package messaging

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

const allTopic = "all"

// TopicInfo describes one open topic as seen by the caller.
type TopicInfo struct {
	Name       string `json:"name"`
	CreatedBy  string `json:"createdBy"`
	Subscribed bool   `json:"subscribed"`
}

// ListTopics returns the open topics of space, all first, then by name.
func (s *Service) ListTopics(ctx context.Context, p auth.Principal, space string) ([]TopicInfo, error) {
	m, err := s.spaces.Resolve(ctx, s.db, p, space)
	if err != nil {
		return nil, err
	}
	return s.TopicsFor(ctx, m)
}

// TopicsFor is ListTopics for the already resolved membership m.
func (s *Service) TopicsFor(ctx context.Context, m spaces.Membership) ([]TopicInfo, error) {
	return listTopics(ctx, s.db, m.SpaceID, m.AgentID, m.Joined())
}

// CreateTopic opens topic name in space and subscribes the creator.
func (s *Service) CreateTopic(ctx context.Context, p auth.Principal, space, name string) error {
	if err := domain.ValidName("topic", name); err != nil {
		return err
	}
	if name == allTopic {
		return apperr.New(apperr.Conflict, "topic %s is already open", name)
	}
	return db.InTx(ctx, s.db, func(tx *sql.Tx) error {
		m, err := s.spaces.Resolve(ctx, tx, p, space)
		if err != nil {
			return err
		}
		if _, found, err := openTopic(ctx, tx, m.SpaceID, name); err != nil || found {
			if err == nil {
				err = apperr.New(apperr.Conflict, "topic %s is already open", name)
			}
			return err
		}
		id, err := insertTopic(ctx, tx, m.SpaceID, name, m.AgentID, db.Timestamp(s.now()))
		if err != nil || !m.Joined() {
			return err // only participants are subscribed; an admin that has not joined is not
		}
		return subscribe(ctx, tx, id, m.AgentID)
	})
}

// SendTopic delivers body to the topic's recipients except the sender and returns a
// receipt per recipient, sorted by name.
func (s *Service) SendTopic(ctx context.Context, p auth.Principal, space, topic, body string, critical bool) ([]inbox.Receipt, error) {
	return inbox.InTx(ctx, s.db, s.ib, func(tx db.DBTX) ([]inbox.Entry, error) {
		m, err := s.spaces.Resolve(ctx, tx, p, space)
		if err != nil {
			return nil, err
		}
		if err := checkMessage(m, body, critical); err != nil {
			return nil, err
		}
		t, err := requireTopic(ctx, tx, m, topic)
		if err != nil {
			return nil, err
		}
		recipients, err := s.topicRecipients(ctx, tx, m, t)
		if err != nil {
			return nil, err
		}
		proto := inbox.Entry{Type: inbox.TopicMessage, Critical: critical, Topic: topic, Body: body}
		return m.Notify(recipients, proto), nil
	})
}

func (s *Service) topicRecipients(ctx context.Context, tx db.DBTX, m spaces.Membership, t topicRow) ([]int64, error) {
	if t.name != allTopic {
		return subscriberIDs(ctx, tx, t.id, m.AgentID)
	}
	if !m.Leads() {
		return nil, apperr.New(apperr.Forbidden, "only leaders may send to all")
	}
	ids, err := s.spaces.ParticipantIDs(ctx, tx, m.SpaceID)
	if err != nil {
		return nil, err
	}
	return without(ids, m.AgentID), nil
}

// Subscribe subscribes the caller to topic (idempotent).
func (s *Service) Subscribe(ctx context.Context, p auth.Principal, space, topic string) error {
	return s.changeSubscription(ctx, p, space, topic, subscribe)
}

// Unsubscribe unsubscribes the caller from topic (idempotent).
func (s *Service) Unsubscribe(ctx context.Context, p auth.Principal, space, topic string) error {
	return s.changeSubscription(ctx, p, space, topic, unsubscribe)
}

type subscriptionOp func(ctx context.Context, q db.DBTX, topicID, agentID int64) error

func (s *Service) changeSubscription(ctx context.Context, p auth.Principal, space, topic string, op subscriptionOp) error {
	if topic == allTopic {
		return apperr.New(apperr.Invalid, "the all topic cannot be subscribed or unsubscribed")
	}
	return db.InTx(ctx, s.db, func(tx *sql.Tx) error {
		m, err := s.spaces.Resolve(ctx, tx, p, space)
		if err != nil {
			return err
		}
		if err := m.RequireJoined(); err != nil {
			return err
		}
		t, err := requireTopic(ctx, tx, m, topic)
		if err != nil {
			return err
		}
		return op(ctx, tx, t.id, m.AgentID)
	})
}

// CloseTopic closes topic, notifying its subscribers except the closer.
func (s *Service) CloseTopic(ctx context.Context, p auth.Principal, space, topic string) error {
	if topic == allTopic {
		return apperr.New(apperr.Forbidden, "the all topic cannot be closed")
	}
	_, err := inbox.InTx(ctx, s.db, s.ib, func(tx db.DBTX) ([]inbox.Entry, error) {
		m, err := s.spaces.Resolve(ctx, tx, p, space)
		if err != nil {
			return nil, err
		}
		t, err := requireTopic(ctx, tx, m, topic)
		if err != nil {
			return nil, err
		}
		if t.createdBy != m.AgentID && !m.Leads() {
			return nil, apperr.New(apperr.Forbidden, "only the creator or a leader may close topic %s", topic)
		}
		recipients, err := subscriberIDs(ctx, tx, t.id, m.AgentID)
		if err != nil {
			return nil, err
		}
		if err := closeTopic(ctx, tx, t.id, db.Timestamp(s.now())); err != nil {
			return nil, err
		}
		return m.Notify(recipients, inbox.Entry{Type: inbox.TopicClosed, Topic: topic}), nil
	})
	return err
}

func requireTopic(ctx context.Context, q db.DBTX, m spaces.Membership, name string) (topicRow, error) {
	t, found, err := openTopic(ctx, q, m.SpaceID, name)
	if err != nil {
		return topicRow{}, err
	}
	if !found {
		return topicRow{}, apperr.New(apperr.NotFound, "topic %s not found in space %s", name, m.SpaceName)
	}
	return t, nil
}
