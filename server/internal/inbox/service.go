// Package inbox stores the durable per-agent inbox and streams it to agents.
package inbox

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// MaxLimit is the most entries one read returns.
const MaxLimit = 500

const (
	defaultLimit        = 100
	defaultHeartbeat    = 20 * time.Second
	defaultFrameTimeout = 30 * time.Second
)

// publisher is the broker surface the inbox needs.
type publisher interface {
	Publish(topic string, payload []byte)
	Subscribe(topic string) (<-chan []byte, func())
}

// Service stores, reads and acknowledges inbox entries and pushes them live.
type Service struct {
	root         context.Context // ends every Wait at shutdown
	db           *sql.DB
	pub          publisher
	creds        credentials
	now          func() time.Time
	heartbeat    time.Duration
	frameTimeout time.Duration // longest write of one stream frame
	presence     *presence
}

// NewService builds the inbox; root ends every pending Wait, creds re-validates the token
// of an open stream or a pending Wait, now stamps CreatedAt on inserted entries.
func NewService(root context.Context, conn *sql.DB, pub publisher, creds credentials, now func() time.Time) *Service {
	return &Service{root: root, db: conn, pub: pub, creds: creds, now: now,
		heartbeat: defaultHeartbeat, frameTimeout: defaultFrameTimeout, presence: newPresence(now)}
}

func topic(agentID int64) string { return fmt.Sprintf("inbox/%d", agentID) }

// insert stores es through q, marking each edge (isEdge), and returns copies with
// CreatedAt and ID set.
func (s *Service) insert(ctx context.Context, q db.DBTX, es []Entry) ([]Entry, error) {
	createdAt := db.Timestamp(s.now())
	out := make([]Entry, 0, len(es))
	for _, e := range es {
		e.CreatedAt, e.Class = createdAt, ClassOf(e.Type, e.Critical)
		edge, err := isEdge(ctx, q, e)
		if err != nil {
			return nil, err
		}
		id, err := insertEntry(ctx, q, e, edge)
		if err != nil {
			return nil, err
		}
		e.ID = id
		out = append(out, e)
	}
	return out, nil
}

// publish sends one empty wake-up signal to topic inbox/<agentID> per distinct
// agent in es. Call it after the inserting tx commits.
func (s *Service) publish(es []Entry) {
	seen := make(map[int64]struct{}, len(es))
	for _, e := range es {
		if _, ok := seen[e.AgentID]; ok {
			continue
		}
		seen[e.AgentID] = struct{}{}
		s.pub.Publish(topic(e.AgentID), nil)
	}
}

// Read returns p's entries with id > after in id order; members see only their space.
// limit must be 1..500 (PageSize resolves an optional one).
func (s *Service) Read(ctx context.Context, p auth.Principal, after int64, limit int) ([]Entry, error) {
	if err := checkLimit(limit); err != nil {
		return nil, err
	}
	return selectEntries(ctx, s.db, p.AgentID, after, spaceFilter(p), limit)
}

// Ack deletes p's entries with id <= upTo (members: only their space) and
// returns how many were deleted. Acked entries are gone for good.
func (s *Service) Ack(ctx context.Context, p auth.Principal, upTo int64) (int64, error) {
	return ackEntries(ctx, s.db, p.AgentID, upTo, spaceFilter(p))
}

// DeleteSpaceEntries drops every entry of agentID in spaceID through q.
func (s *Service) DeleteSpaceEntries(ctx context.Context, q db.DBTX, agentID, spaceID int64) error {
	return deleteSpaceEntries(ctx, q, agentID, spaceID)
}

// Count returns how many entries wait in p's mailbox scope; it never marks or acks.
func (s *Service) Count(ctx context.Context, p auth.Principal) (int, error) {
	return countEntries(ctx, s.db, p.AgentID, spaceFilter(p))
}

// PageSize turns the optional limit argument into a page size: 100 when absent,
// otherwise 1..500.
func PageSize(limit *int) (int, error) {
	if limit == nil {
		return defaultLimit, nil
	}
	if err := checkLimit(*limit); err != nil {
		return 0, err
	}
	return *limit, nil
}

func checkLimit(limit int) error {
	if limit < 1 || limit > MaxLimit {
		return apperr.New(apperr.Invalid, "limit must be 1..%d", MaxLimit)
	}
	return nil
}

// spaceFilter is the only space p's inbox covers; 0 means every space.
func spaceFilter(p auth.Principal) int64 {
	if p.Kind == auth.Member {
		return p.SpaceID
	}
	return 0
}

// Wake sends the wake-up signal to each agent's streams and waits, which re-validate their
// token. Call it after a transaction committed that deleted the agent's tokens or
// participations, so a dead credential does not hold a slot until the next heartbeat.
func (s *Service) Wake(agentIDs ...int64) {
	for _, id := range agentIDs {
		s.pub.Publish(topic(id), nil)
	}
}
