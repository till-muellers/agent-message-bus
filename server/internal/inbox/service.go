// Package inbox stores the durable per-agent inbox and streams it to agents.
package inbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

const (
	defaultLimit     = 100
	maxLimit         = 500
	defaultHeartbeat = 20 * time.Second
)

const insertSQL = `INSERT INTO inbox (agent_id, space_id, type, from_name, critical,
	topic, blackboard, thread_id, thread_title, body, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// selectSQL takes agent_id, after, space filter (twice; 0 = all spaces), limit.
const selectSQL = `SELECT i.id, i.agent_id, i.space_id, s.name, i.type, i.from_name, i.critical,
	COALESCE(i.topic, ''), COALESCE(i.blackboard, ''), COALESCE(i.thread_id, 0),
	COALESCE(i.thread_title, ''), COALESCE(i.body, ''), i.created_at
	FROM inbox i JOIN spaces s ON s.id = i.space_id
	WHERE i.agent_id = ? AND i.id > ? AND (? = 0 OR i.space_id = ?)
	ORDER BY i.id LIMIT ?`

// ackSQL takes agent_id, upTo, space filter (twice; 0 = all spaces).
const ackSQL = `DELETE FROM inbox WHERE agent_id = ? AND id <= ? AND (? = 0 OR space_id = ?)`

// publisher is the broker surface the inbox needs.
type publisher interface {
	Publish(topic string, payload []byte)
	Subscribe(topic string) (<-chan []byte, func())
}

// Service stores, reads and acknowledges inbox entries and pushes them live.
type Service struct {
	db        *sql.DB
	pub       publisher
	now       func() time.Time
	heartbeat time.Duration
}

// NewService builds the inbox; now stamps CreatedAt on inserted entries.
func NewService(db *sql.DB, pub publisher, now func() time.Time) *Service {
	return &Service{db: db, pub: pub, now: now, heartbeat: defaultHeartbeat}
}

func topic(agentID int64) string { return fmt.Sprintf("inbox/%d", agentID) }

// Insert stores es through tx and returns copies with CreatedAt and ID set.
func (s *Service) Insert(ctx context.Context, tx auth.DBTX, es []Entry) ([]Entry, error) {
	createdAt := s.now().UTC().Format(time.RFC3339Nano)
	out := make([]Entry, 0, len(es))
	for _, e := range es {
		e.CreatedAt = createdAt
		id, err := insertEntry(ctx, tx, e)
		if err != nil {
			return nil, err
		}
		e.ID = id
		out = append(out, e)
	}
	return out, nil
}

func insertEntry(ctx context.Context, tx auth.DBTX, e Entry) (int64, error) {
	res, err := tx.ExecContext(ctx, insertSQL, e.AgentID, e.SpaceID, string(e.Type), e.From, e.Critical,
		nullString(e.Topic), nullString(e.Blackboard), sql.NullInt64{Int64: e.ThreadID, Valid: e.ThreadID != 0},
		nullString(e.ThreadTitle), nullString(e.Body), e.CreatedAt)
	if err != nil {
		return 0, fmt.Errorf("inserting inbox entry for agent %d: %w", e.AgentID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("reading inbox entry id: %w", err)
	}
	return id, nil
}

func nullString(v string) sql.NullString { return sql.NullString{String: v, Valid: v != ""} }

// Publish pushes each entry to topic inbox/<agentID>. Call it after the inserting tx commits.
func (s *Service) Publish(es []Entry) {
	for _, e := range es {
		payload, err := json.Marshal(e)
		if err != nil {
			slog.Error("encoding inbox entry", "id", e.ID, "err", err)
			continue
		}
		s.pub.Publish(topic(e.AgentID), payload)
	}
}

// Read returns p's entries with id > after in id order; members see only their space.
// limit 0 means 100; otherwise it must be 1..500.
func (s *Service) Read(ctx context.Context, p auth.Principal, after int64, limit int) ([]Entry, error) {
	if err := requireAgent(p); err != nil {
		return nil, err
	}
	limit, err := pageSize(limit)
	if err != nil {
		return nil, err
	}
	space := spaceFilter(p)
	rows, err := s.db.QueryContext(ctx, selectSQL, p.AgentID, after, space, space, limit)
	if err != nil {
		return nil, fmt.Errorf("reading inbox of agent %d: %w", p.AgentID, err)
	}
	defer func() { _ = rows.Close() }()
	return scanEntries(rows)
}

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	es := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.AgentID, &e.SpaceID, &e.Space, &e.Type, &e.From, &e.Critical,
			&e.Topic, &e.Blackboard, &e.ThreadID, &e.ThreadTitle, &e.Body, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning inbox entry: %w", err)
		}
		es = append(es, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating inbox entries: %w", err)
	}
	return es, nil
}

// Ack deletes p's entries with id <= upTo (members: only their space) and
// returns how many were deleted. Acked entries are gone for good.
func (s *Service) Ack(ctx context.Context, p auth.Principal, upTo int64) (int64, error) {
	if err := requireAgent(p); err != nil {
		return 0, err
	}
	space := spaceFilter(p)
	res, err := s.db.ExecContext(ctx, ackSQL, p.AgentID, upTo, space, space)
	if err != nil {
		return 0, fmt.Errorf("acking inbox of agent %d: %w", p.AgentID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("counting acked inbox entries: %w", err)
	}
	return n, nil
}

func requireAgent(p auth.Principal) error {
	if p.Kind == auth.Admin {
		return apperr.New(apperr.Forbidden, "admin token has no inbox; mint a leader with bus_mint_leader")
	}
	return nil
}

func pageSize(limit int) (int, error) {
	if limit == 0 {
		return defaultLimit, nil
	}
	if limit < 1 || limit > maxLimit {
		return 0, apperr.New(apperr.Invalid, "limit must be 1..%d", maxLimit)
	}
	return limit, nil
}

// spaceFilter is the only space p's inbox covers; 0 means every space.
func spaceFilter(p auth.Principal) int64 {
	if p.Kind == auth.Member {
		return p.SpaceID
	}
	return 0
}
