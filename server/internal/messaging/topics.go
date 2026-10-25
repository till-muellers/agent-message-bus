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
	m, err := s.spaces.Resolve(ctx, p, space)
	if err != nil {
		return nil, err
	}
	return listTopics(ctx, s.db, m.SpaceID, m.AgentID)
}

// CreateTopic opens topic name in space and subscribes the creator.
func (s *Service) CreateTopic(ctx context.Context, p auth.Principal, space, name string) error {
	if err := domain.ValidName("topic", name); err != nil {
		return err
	}
	if name == allTopic {
		return apperr.New(apperr.Conflict, "topic %s is already open", name)
	}
	return s.inTx(ctx, p, space, func(tx *sql.Tx, m spaces.Membership) ([]inbox.Entry, error) {
		if _, found, err := openTopic(ctx, tx, m.SpaceID, name); err != nil || found {
			if err == nil {
				err = apperr.New(apperr.Conflict, "topic %s is already open", name)
			}
			return nil, err
		}
		id, err := insertTopic(ctx, tx, m.SpaceID, name, m.AgentID, s.nowText())
		if err != nil {
			return nil, err
		}
		return nil, subscribe(ctx, tx, id, m.AgentID)
	})
}

// SendTopic delivers body to the topic's recipients except the sender.
func (s *Service) SendTopic(ctx context.Context, p auth.Principal, space, topic, body string, critical bool) error {
	return s.inTx(ctx, p, space, func(tx *sql.Tx, m spaces.Membership) ([]inbox.Entry, error) {
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
		return fanOut(m, recipients, proto), nil
	})
}

func (s *Service) topicRecipients(ctx context.Context, tx *sql.Tx, m spaces.Membership, t topicRow) ([]int64, error) {
	if t.name != allTopic {
		return subscriberIDs(ctx, tx, t.id, m.AgentID)
	}
	if m.Role != spaces.RoleLeader {
		return nil, apperr.New(apperr.Forbidden, "only leaders may send to all")
	}
	return participantIDs(ctx, tx, m.SpaceID, m.AgentID)
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
	return s.inTx(ctx, p, space, func(tx *sql.Tx, m spaces.Membership) ([]inbox.Entry, error) {
		t, err := requireTopic(ctx, tx, m, topic)
		if err != nil {
			return nil, err
		}
		return nil, op(ctx, tx, t.id, m.AgentID)
	})
}

// CloseTopic closes topic, notifying its subscribers except the closer.
func (s *Service) CloseTopic(ctx context.Context, p auth.Principal, space, topic string) error {
	if topic == allTopic {
		return apperr.New(apperr.Forbidden, "the all topic cannot be closed")
	}
	return s.inTx(ctx, p, space, func(tx *sql.Tx, m spaces.Membership) ([]inbox.Entry, error) {
		t, err := requireTopic(ctx, tx, m, topic)
		if err != nil {
			return nil, err
		}
		if t.createdBy != m.AgentID && m.Role != spaces.RoleLeader {
			return nil, apperr.New(apperr.Forbidden, "only the creator or a leader may close topic %s", topic)
		}
		recipients, err := subscriberIDs(ctx, tx, t.id, m.AgentID)
		if err != nil {
			return nil, err
		}
		if err := closeTopic(ctx, tx, t.id, s.nowText()); err != nil {
			return nil, err
		}
		return fanOut(m, recipients, inbox.Entry{Type: inbox.TopicClosed, Topic: topic}), nil
	})
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
