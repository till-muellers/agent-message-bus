package inbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

const insertSQL = `INSERT INTO inbox (agent_id, space_id, type, from_name, critical,
	topic, blackboard, thread_id, thread_title, body, created_at, edge)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

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

// countSQL takes agent_id, space filter (twice; 0 = all spaces).
const countSQL = `SELECT COUNT(*) FROM inbox WHERE agent_id = ? AND (? = 0 OR space_id = ?)`

const deleteSpaceSQL = `DELETE FROM inbox WHERE agent_id = ? AND space_id = ?`

// scopeOccupiedSQL takes agent_id, space_id, agent_id: does the agent's mailbox scope
// hold an entry? Members' scope is one space; leaders' and admins' every space.
const scopeOccupiedSQL = `SELECT EXISTS (SELECT 1 FROM inbox WHERE agent_id = ?
	AND (space_id = ? OR (SELECT kind FROM agents WHERE id = ?) <> 'member'))`

// markNoticedSQL takes agent_id, space filter (twice; 0 = all spaces).
const markNoticedSQL = `UPDATE inbox SET noticed = 1
	WHERE agent_id = ? AND (? = 0 OR space_id = ?) AND edge = 1 AND noticed = 0`

// markEntriesNoticedSQL takes agent_id and a JSON array of entry ids; it marks those of
// them that are unnoticed edges.
const markEntriesNoticedSQL = `UPDATE inbox SET noticed = 1
	WHERE agent_id = ? AND edge = 1 AND noticed = 0 AND id IN (SELECT value FROM json_each(?))`

// waitingSQL takes agent_id, space filter (twice; 0 = all spaces); one row per space.
const waitingSQL = `SELECT s.name, COUNT(*), SUM(i.critical)
	FROM inbox i JOIN spaces s ON s.id = i.space_id
	WHERE i.agent_id = ? AND (? = 0 OR i.space_id = ?)
	GROUP BY s.name ORDER BY s.name`

func insertEntry(ctx context.Context, q db.DBTX, e Entry, edge bool) (int64, error) {
	id, err := db.InsertID(ctx, q, insertSQL, e.AgentID, e.SpaceID, string(e.Type), e.From, e.Critical,
		nullString(e.Topic), nullString(e.Blackboard), sql.NullInt64{Int64: e.ThreadID, Valid: e.ThreadID != 0},
		nullString(e.ThreadTitle), nullString(e.Body), e.CreatedAt, edge)
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

// countEntries counts agentID's entries in space (0 = all).
func countEntries(ctx context.Context, q db.DBTX, agentID, space int64) (int, error) {
	var n int
	if err := q.QueryRowContext(ctx, countSQL, agentID, space, space).Scan(&n); err != nil {
		return 0, fmt.Errorf("counting inbox of agent %d: %w", agentID, err)
	}
	return n, nil
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

func scopeOccupied(ctx context.Context, q db.DBTX, agentID, spaceID int64) (bool, error) {
	var occupied bool
	if err := q.QueryRowContext(ctx, scopeOccupiedSQL, agentID, spaceID, agentID).Scan(&occupied); err != nil {
		return false, fmt.Errorf("checking mailbox scope of agent %d in space %d: %w", agentID, spaceID, err)
	}
	return occupied, nil
}

// markNoticed marks agentID's unnoticed edges in space (0 = all) noticed and returns how many.
func markNoticed(ctx context.Context, q db.DBTX, agentID, space int64) (int64, error) {
	res, err := q.ExecContext(ctx, markNoticedSQL, agentID, space, space)
	if err != nil {
		return 0, fmt.Errorf("marking inbox edges of agent %d noticed: %w", agentID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("counting noticed inbox edges: %w", err)
	}
	return n, nil
}

// markEntriesNoticed marks the unnoticed edges among agentID's entries ids noticed.
func markEntriesNoticed(ctx context.Context, q db.DBTX, agentID int64, ids []int64) error {
	list, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("encoding inbox entry ids: %w", err)
	}
	if _, err := q.ExecContext(ctx, markEntriesNoticedSQL, agentID, string(list)); err != nil {
		return fmt.Errorf("marking returned inbox edges of agent %d noticed: %w", agentID, err)
	}
	return nil
}

// waitingIn counts agentID's entries in space (0 = all) and names their spaces.
func waitingIn(ctx context.Context, q db.DBTX, agentID, space int64) (Notice, error) {
	rows, err := q.QueryContext(ctx, waitingSQL, agentID, space, space)
	if err != nil {
		return Notice{}, fmt.Errorf("counting waiting inbox entries of agent %d: %w", agentID, err)
	}
	defer func() { _ = rows.Close() }()
	n := Notice{Spaces: []string{}}
	for rows.Next() {
		var name string
		var waiting, critical int
		if err := rows.Scan(&name, &waiting, &critical); err != nil {
			return Notice{}, fmt.Errorf("scanning waiting inbox entries: %w", err)
		}
		n.Waiting, n.Critical, n.Spaces = n.Waiting+waiting, n.Critical+critical, append(n.Spaces, name)
	}
	if err := rows.Err(); err != nil {
		return Notice{}, fmt.Errorf("iterating waiting inbox entries: %w", err)
	}
	return n, nil
}

// recipientNamesSQL takes a JSON array of agent ids; their names, sorted.
const recipientNamesSQL = `SELECT id, name FROM agents WHERE id IN (SELECT value FROM json_each(?)) ORDER BY name`

// recipient is an agent an entry was delivered to.
type recipient struct {
	id   int64
	name string
}

// recipientNames returns the agents ids, sorted by name.
func recipientNames(ctx context.Context, q db.DBTX, ids []int64) ([]recipient, error) {
	list, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("encoding recipient ids: %w", err)
	}
	rows, err := q.QueryContext(ctx, recipientNamesSQL, string(list))
	if err != nil {
		return nil, fmt.Errorf("reading recipient names: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []recipient{}
	for rows.Next() {
		var r recipient
		if err := rows.Scan(&r.id, &r.name); err != nil {
			return nil, fmt.Errorf("scanning recipient: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating recipients: %w", err)
	}
	return out, nil
}
