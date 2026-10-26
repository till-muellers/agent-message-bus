package inbox

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
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

// matchSQL takes agent_id, after, space filter (twice), from (twice; "" = any sender),
// thread id (twice; 0 = any thread), limit.
const matchSQL = `SELECT i.id, i.agent_id, i.space_id, s.name, i.type, i.from_name, i.critical,
	COALESCE(i.topic, ''), COALESCE(i.blackboard, ''), COALESCE(i.thread_id, 0),
	COALESCE(i.thread_title, ''), COALESCE(i.body, ''), i.created_at
	FROM inbox i JOIN spaces s ON s.id = i.space_id
	WHERE i.agent_id = ? AND i.id > ? AND (? = 0 OR i.space_id = ?)
	AND (? = '' OR i.from_name = ?) AND (? = 0 OR i.thread_id = ?)
	ORDER BY i.id LIMIT ?`

// ackSQL takes agent_id, upTo, space filter (twice; 0 = all spaces).
const ackSQL = `DELETE FROM inbox WHERE agent_id = ? AND id <= ? AND (? = 0 OR space_id = ?)`

const deleteSpaceSQL = `DELETE FROM inbox WHERE agent_id = ? AND space_id = ?`

func insertEntry(ctx context.Context, q db.DBTX, e Entry) (int64, error) {
	id, err := db.InsertID(ctx, q, insertSQL, e.AgentID, e.SpaceID, string(e.Type), e.From, e.Critical,
		nullString(e.Topic), nullString(e.Blackboard), sql.NullInt64{Int64: e.ThreadID, Valid: e.ThreadID != 0},
		nullString(e.ThreadTitle), nullString(e.Body), e.CreatedAt)
	if err != nil {
		return 0, fmt.Errorf("inserting inbox entry for agent %d: %w", e.AgentID, err)
	}
	return id, nil
}

func nullString(v string) sql.NullString { return sql.NullString{String: v, Valid: v != ""} }

func selectEntries(ctx context.Context, q db.DBTX, agentID, after, space int64, limit int) ([]Entry, error) {
	rows, err := q.QueryContext(ctx, selectSQL, agentID, after, space, space, limit)
	if err != nil {
		return nil, fmt.Errorf("reading inbox of agent %d: %w", agentID, err)
	}
	defer func() { _ = rows.Close() }()
	return scanEntries(rows)
}

// selectMatching reads agentID's entries in space (0 = all) selected by q, at most waitLimit.
func selectMatching(ctx context.Context, q db.DBTX, agentID, space int64, w WaitQuery) ([]Entry, error) {
	rows, err := q.QueryContext(ctx, matchSQL, agentID, w.After, space, space, w.From, w.From,
		w.ThreadID, w.ThreadID, waitLimit)
	if err != nil {
		return nil, fmt.Errorf("reading matching inbox entries of agent %d: %w", agentID, err)
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
		e.Class = ClassOf(e.Type, e.Critical)
		es = append(es, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating inbox entries: %w", err)
	}
	return es, nil
}

func ackEntries(ctx context.Context, q db.DBTX, agentID, upTo, space int64) (int64, error) {
	res, err := q.ExecContext(ctx, ackSQL, agentID, upTo, space, space)
	if err != nil {
		return 0, fmt.Errorf("acking inbox of agent %d: %w", agentID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("counting acked inbox entries: %w", err)
	}
	return n, nil
}

func deleteSpaceEntries(ctx context.Context, q db.DBTX, agentID, spaceID int64) error {
	if _, err := q.ExecContext(ctx, deleteSpaceSQL, agentID, spaceID); err != nil {
		return fmt.Errorf("deleting inbox of agent %d in space %d: %w", agentID, spaceID, err)
	}
	return nil
}
