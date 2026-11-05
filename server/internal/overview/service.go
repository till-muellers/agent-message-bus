// Package overview composes read-only views: one space at a glance (bus_space_overview)
// and the web GUI state, from the spaces, messaging, blackboards, invites and inbox
// services. It owns no table.
package overview

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/blackboards"
	"github.com/till-muellers/agent-message-bus/server/internal/invites"
	"github.com/till-muellers/agent-message-bus/server/internal/messaging"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// RecentThreads is how many of the most recently active threads an overview shows.
const RecentThreads = 10

type resolver interface {
	Resolve(ctx context.Context, q db.DBTX, p auth.Principal, space string) (spaces.Membership, error)
}

type spaceReader interface {
	Whoami(ctx context.Context, p auth.Principal) (spaces.WhoamiResult, error)
	Listing(ctx context.Context, p auth.Principal) ([]spaces.Listed, error)
	ParticipantsFor(ctx context.Context, m spaces.Membership) ([]spaces.Participant, error)
}

type topicLister interface {
	TopicsFor(ctx context.Context, m spaces.Membership) ([]messaging.TopicInfo, error)
}

type boardReader interface {
	BoardsFor(ctx context.Context, m spaces.Membership) ([]blackboards.BoardInfo, error)
	RecentThreadsFor(ctx context.Context, m spaces.Membership, limit int) ([]blackboards.RecentThread, error)
}

type inviteReader interface {
	CountOpen(ctx context.Context, spaceIDs []int64) (map[int64]int, error)
	ListOpen(ctx context.Context, spaceID int64) ([]invites.Pending, error)
}

type mailbox interface {
	Count(ctx context.Context, p auth.Principal) (int, error)
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

// Service builds overviews and the GUI state; it only reads.
type Service struct {
	q   db.DBTX
	r   resolver
	sp  spaceReader
	tp  topicLister
	bb  boardReader
	inv inviteReader
	ib  mailbox
}

// Deps are the reads an overview Service composes; DB is the database the membership is
// resolved through.
type Deps struct {
	DB       db.DBTX
	Resolver resolver
	Spaces   spaceReader
	Topics   topicLister
	Boards   boardReader
	Invites  inviteReader
	Inbox    mailbox
}

// NewService builds an overview Service over the feature services' reads.
func NewService(d Deps) *Service {
	return &Service{q: d.DB, r: d.Resolver, sp: d.Spaces, tp: d.Topics, bb: d.Boards, inv: d.Invites, ib: d.Inbox}
}

// Space returns the overview of space for p. It resolves the membership once; the reads
// run outside a transaction, like the list tools they mirror.
func (s *Service) Space(ctx context.Context, p auth.Principal, space string) (Overview, error) {
	m, err := s.r.Resolve(ctx, s.q, p, space)
	if err != nil {
		return Overview{}, err
	}
	return s.of(ctx, m)
}

// of builds the overview of m's space: participants, topics, boards and recent threads.
func (s *Service) of(ctx context.Context, m spaces.Membership) (Overview, error) {
	o := Overview{Space: m.SpaceName, You: You{Name: m.AgentName}}
	if m.Joined() {
		role := m.Role
		o.You.Role = &role
	}
	var err error
	if o.Participants, err = s.sp.ParticipantsFor(ctx, m); err != nil {
		return Overview{}, err
	}
	if o.Topics, err = s.tp.TopicsFor(ctx, m); err != nil {
		return Overview{}, err
	}
	if o.Blackboards, err = s.bb.BoardsFor(ctx, m); err != nil {
		return Overview{}, err
	}
	if o.RecentThreads, err = s.bb.RecentThreadsFor(ctx, m, RecentThreads); err != nil {
		return Overview{}, err
	}
	return o, nil
}
