// Package overview composes one read-only view of a space from the spaces, messaging and
// blackboards services. It owns no table.
package overview

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/blackboards"
	"github.com/till-muellers/agent-message-bus/server/internal/messaging"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// RecentThreads is how many of the most recently active threads an overview shows.
const RecentThreads = 10

type resolver interface {
	Resolve(ctx context.Context, p auth.Principal, space string) (spaces.Membership, error)
}

type participantLister interface {
	Participants(ctx context.Context, p auth.Principal, space string) ([]spaces.Participant, error)
}

type topicLister interface {
	ListTopics(ctx context.Context, p auth.Principal, space string) ([]messaging.TopicInfo, error)
}

type boardReader interface {
	List(ctx context.Context, p auth.Principal, space string) ([]blackboards.BoardInfo, error)
	RecentThreads(ctx context.Context, p auth.Principal, space string, limit int) ([]blackboards.RecentThread, error)
}

// Overview is one space at a glance, as seen by the caller.
type Overview struct {
	Space         string                     `json:"space"`
	You           You                        `json:"you"`
	Participants  []spaces.Participant       `json:"participants"`
	Topics        []messaging.TopicInfo      `json:"topics"`
	Blackboards   []blackboards.BoardInfo    `json:"blackboards"`
	RecentThreads []blackboards.RecentThread `json:"recentThreads"`
}

// You is the caller; Role is nil for an admin that has not joined the space.
type You struct {
	Name string       `json:"name"`
	Role *spaces.Role `json:"role"`
}

// Service builds overviews; it only reads.
type Service struct {
	r  resolver
	sp participantLister
	tp topicLister
	bb boardReader
}

// NewService builds an overview Service over the feature services' reads.
func NewService(r resolver, sp participantLister, tp topicLister, bb boardReader) *Service {
	return &Service{r: r, sp: sp, tp: tp, bb: bb}
}

// Space returns the overview of space for p. The reads run outside a transaction, like
// the list tools they mirror.
func (s *Service) Space(ctx context.Context, p auth.Principal, space string) (Overview, error) {
	m, err := s.r.Resolve(ctx, p, space)
	if err != nil {
		return Overview{}, err
	}
	o := Overview{Space: m.SpaceName, You: You{Name: m.AgentName}}
	if m.Joined() {
		role := m.Role
		o.You.Role = &role
	}
	if err := s.fill(ctx, p, &o); err != nil {
		return Overview{}, err
	}
	return o, nil
}

// fill reads the participants, topics, boards and recent threads of o.Space into o.
func (s *Service) fill(ctx context.Context, p auth.Principal, o *Overview) error {
	var err error
	if o.Participants, err = s.sp.Participants(ctx, p, o.Space); err != nil {
		return err
	}
	if o.Topics, err = s.tp.ListTopics(ctx, p, o.Space); err != nil {
		return err
	}
	if o.Blackboards, err = s.bb.List(ctx, p, o.Space); err != nil {
		return err
	}
	o.RecentThreads, err = s.bb.RecentThreads(ctx, p, o.Space, RecentThreads)
	return err
}
