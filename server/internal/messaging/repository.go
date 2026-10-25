package messaging

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

type topicRow struct {
	id, createdBy int64
	name          string
}

func participantID(ctx context.Context, q db.DBTX, spaceID int64, name string) (int64, bool, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT a.id FROM participants p JOIN agents a ON a.id = p.agent_id
		WHERE p.space_id = ? AND a.name = ?`, spaceID, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("looking up participant %s: %w", name, err)
	}
	return id, true, nil
}

func openTopic(ctx context.Context, q db.DBTX, spaceID int64, name string) (topicRow, bool, error) {
	t := topicRow{name: name}
	err := q.QueryRowContext(ctx, `SELECT id, created_by FROM topics
		WHERE space_id = ? AND name = ? AND closed_at IS NULL`, spaceID, name).Scan(&t.id, &t.createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		return topicRow{}, false, nil
	}
	if err != nil {
		return topicRow{}, false, fmt.Errorf("looking up topic %s: %w", name, err)
	}
	return t, true, nil
}

func insertTopic(ctx context.Context, q db.DBTX, spaceID int64, name string, by int64, at string) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO topics (space_id, name, created_by, created_at)
		VALUES (?, ?, ?, ?)`, spaceID, name, by, at)
	if err != nil {
		return 0, fmt.Errorf("inserting topic %s: %w", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("reading topic id: %w", err)
	}
	return id, nil
}

func subscribe(ctx context.Context, q db.DBTX, topicID, agentID int64) error {
	_, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO topic_subscriptions (topic_id, agent_id) VALUES (?, ?)`,
		topicID, agentID)
	if err != nil {
		return fmt.Errorf("subscribing agent %d to topic %d: %w", agentID, topicID, err)
	}
	return nil
}

func unsubscribe(ctx context.Context, q db.DBTX, topicID, agentID int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM topic_subscriptions WHERE topic_id = ? AND agent_id = ?`,
		topicID, agentID)
	if err != nil {
		return fmt.Errorf("unsubscribing agent %d from topic %d: %w", agentID, topicID, err)
	}
	return nil
}

func closeTopic(ctx context.Context, q db.DBTX, topicID int64, at string) error {
	if _, err := q.ExecContext(ctx, `UPDATE topics SET closed_at = ? WHERE id = ?`, at, topicID); err != nil {
		return fmt.Errorf("closing topic %d: %w", topicID, err)
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM topic_subscriptions WHERE topic_id = ?`, topicID); err != nil {
		return fmt.Errorf("deleting subscriptions of topic %d: %w", topicID, err)
	}
	return nil
}

func subscriberIDs(ctx context.Context, q db.DBTX, topicID, except int64) ([]int64, error) {
	return queryIDs(ctx, q, `SELECT agent_id FROM topic_subscriptions
		WHERE topic_id = ? AND agent_id != ? ORDER BY agent_id`, topicID, except)
}

func participantIDs(ctx context.Context, q db.DBTX, spaceID, except int64) ([]int64, error) {
	return queryIDs(ctx, q, `SELECT agent_id FROM participants
		WHERE space_id = ? AND agent_id != ? ORDER BY agent_id`, spaceID, except)
}

func queryIDs(ctx context.Context, q db.DBTX, query string, args ...any) ([]int64, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying recipients: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning recipient: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating recipients: %w", err)
	}
	return ids, nil
}

func listTopics(ctx context.Context, q db.DBTX, spaceID, agentID int64) ([]TopicInfo, error) {
	rows, err := q.QueryContext(ctx, `SELECT t.name, a.name,
		t.name = 'all' OR EXISTS (SELECT 1 FROM topic_subscriptions s WHERE s.topic_id = t.id AND s.agent_id = ?)
		FROM topics t JOIN agents a ON a.id = t.created_by
		WHERE t.space_id = ? AND t.closed_at IS NULL
		ORDER BY t.name != 'all', t.name`, agentID, spaceID)
	if err != nil {
		return nil, fmt.Errorf("listing topics: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ts := []TopicInfo{}
	for rows.Next() {
		var t TopicInfo
		if err := rows.Scan(&t.Name, &t.CreatedBy, &t.Subscribed); err != nil {
			return nil, fmt.Errorf("scanning topic: %w", err)
		}
		ts = append(ts, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating topics: %w", err)
	}
	return ts, nil
}
