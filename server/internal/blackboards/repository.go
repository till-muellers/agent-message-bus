package blackboards

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

type board struct {
	id      int64
	name    string
	ownerID int64
}

type thread struct {
	id          int64
	board       string
	title       string
	createdByID int64
	createdBy   string
}

func findBoard(ctx context.Context, q db.DBTX, spaceID int64, name string) (board, bool, error) {
	b := board{name: name}
	err := q.QueryRowContext(ctx, `SELECT id, owner_id FROM blackboards WHERE space_id = ? AND name = ?`,
		spaceID, name).Scan(&b.id, &b.ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return board{}, false, nil
	}
	if err != nil {
		return board{}, false, fmt.Errorf("finding blackboard %s: %w", name, err)
	}
	return b, true, nil
}

func insertBoard(ctx context.Context, q db.DBTX, spaceID int64, name string, ownerID int64, at string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO blackboards (space_id, name, owner_id, created_at) VALUES (?, ?, ?, ?)`,
		spaceID, name, ownerID, at)
	if err != nil {
		return fmt.Errorf("inserting blackboard %s: %w", name, err)
	}
	return nil
}

func deleteBoard(ctx context.Context, q db.DBTX, id int64) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM blackboards WHERE id = ?`, id); err != nil {
		return fmt.Errorf("deleting blackboard %d: %w", id, err)
	}
	return nil
}

func listBoards(ctx context.Context, q db.DBTX, spaceID int64) ([]BoardInfo, error) {
	rows, err := q.QueryContext(ctx, `SELECT b.name, a.name, (SELECT COUNT(*) FROM threads t WHERE t.blackboard_id = b.id)
		FROM blackboards b JOIN agents a ON a.id = b.owner_id WHERE b.space_id = ? ORDER BY b.name`, spaceID)
	if err != nil {
		return nil, fmt.Errorf("listing blackboards: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []BoardInfo{}
	for rows.Next() {
		var b BoardInfo
		if err := rows.Scan(&b.Name, &b.Owner, &b.Threads); err != nil {
			return nil, fmt.Errorf("scanning blackboard: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating blackboards: %w", err)
	}
	return out, nil
}

func ownedBoards(ctx context.Context, q db.DBTX, spaceID, ownerID int64) ([]board, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, name FROM blackboards WHERE space_id = ? AND owner_id = ? ORDER BY id`,
		spaceID, ownerID)
	if err != nil {
		return nil, fmt.Errorf("listing owned blackboards: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []board
	for rows.Next() {
		b := board{ownerID: ownerID}
		if err := rows.Scan(&b.id, &b.name); err != nil {
			return nil, fmt.Errorf("scanning blackboard: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating owned blackboards: %w", err)
	}
	return out, nil
}

func findThread(ctx context.Context, q db.DBTX, spaceID, id int64) (thread, bool, error) {
	t := thread{id: id}
	err := q.QueryRowContext(ctx, `SELECT b.name, t.title, t.created_by, a.name FROM threads t
		JOIN blackboards b ON b.id = t.blackboard_id JOIN agents a ON a.id = t.created_by
		WHERE t.id = ? AND b.space_id = ?`, id, spaceID).Scan(&t.board, &t.title, &t.createdByID, &t.createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		return thread{}, false, nil
	}
	if err != nil {
		return thread{}, false, fmt.Errorf("finding thread %d: %w", id, err)
	}
	return t, true, nil
}

func insertThread(ctx context.Context, q db.DBTX, boardID int64, title string, by int64, at string) (int64, error) {
	id, err := db.InsertID(ctx, q, `INSERT INTO threads (blackboard_id, title, created_by, created_at) VALUES (?, ?, ?, ?)`,
		boardID, title, by, at)
	if err != nil {
		return 0, fmt.Errorf("thread: %w", err)
	}
	return id, nil
}

func insertPost(ctx context.Context, q db.DBTX, threadID, author int64, body, at string) (int64, error) {
	id, err := db.InsertID(ctx, q, `INSERT INTO posts (thread_id, author_id, body, created_at) VALUES (?, ?, ?, ?)`,
		threadID, author, body, at)
	if err != nil {
		return 0, fmt.Errorf("post: %w", err)
	}
	return id, nil
}

func deleteThread(ctx context.Context, q db.DBTX, id int64) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM threads WHERE id = ?`, id); err != nil {
		return fmt.Errorf("deleting thread %d: %w", id, err)
	}
	return nil
}

func listThreads(ctx context.Context, q db.DBTX, boardID int64) ([]ThreadInfo, error) {
	rows, err := q.QueryContext(ctx, `SELECT t.id, t.title, a.name, COUNT(p.id), COALESCE(MAX(p.created_at), t.created_at) AS last
		FROM threads t JOIN agents a ON a.id = t.created_by LEFT JOIN posts p ON p.thread_id = t.id
		WHERE t.blackboard_id = ? GROUP BY t.id ORDER BY last DESC, MAX(p.id) DESC`, boardID)
	if err != nil {
		return nil, fmt.Errorf("listing threads: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []ThreadInfo{}
	for rows.Next() {
		var t ThreadInfo
		if err := rows.Scan(&t.ID, &t.Title, &t.CreatedBy, &t.Posts, &t.LastPostAt); err != nil {
			return nil, fmt.Errorf("scanning thread: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating threads: %w", err)
	}
	return out, nil
}

// recentThreads lists at most limit threads of every board in spaceID, latest post first,
// with agentID's subscription state.
func recentThreads(ctx context.Context, q db.DBTX, spaceID, agentID int64, limit int) ([]RecentThread, error) {
	rows, err := q.QueryContext(ctx, `SELECT t.id, b.name, t.title, a.name, COUNT(p.id),
		COALESCE(MAX(p.created_at), t.created_at) AS last,
		EXISTS (SELECT 1 FROM thread_subscriptions s WHERE s.thread_id = t.id AND s.agent_id = ?)
		FROM threads t JOIN blackboards b ON b.id = t.blackboard_id JOIN agents a ON a.id = t.created_by
		LEFT JOIN posts p ON p.thread_id = t.id
		WHERE b.space_id = ? GROUP BY t.id ORDER BY last DESC, MAX(p.id) DESC LIMIT ?`, agentID, spaceID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing recent threads: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []RecentThread{}
	for rows.Next() {
		var t RecentThread
		if err := rows.Scan(&t.ID, &t.Blackboard, &t.Title, &t.CreatedBy, &t.Posts, &t.LastPostAt, &t.Subscribed); err != nil {
			return nil, fmt.Errorf("scanning recent thread: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating recent threads: %w", err)
	}
	return out, nil
}

func listPosts(ctx context.Context, q db.DBTX, threadID int64) ([]Post, error) {
	rows, err := q.QueryContext(ctx, `SELECT p.id, a.name, p.body, p.created_at FROM posts p
		JOIN agents a ON a.id = p.author_id WHERE p.thread_id = ? ORDER BY p.id`, threadID)
	if err != nil {
		return nil, fmt.Errorf("listing posts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []Post{}
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.Author, &p.Body, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning post: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating posts: %w", err)
	}
	return out, nil
}

func subscribe(ctx context.Context, q db.DBTX, threadID, agentID int64) error {
	_, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO thread_subscriptions (thread_id, agent_id) VALUES (?, ?)`,
		threadID, agentID)
	if err != nil {
		return fmt.Errorf("subscribing to thread %d: %w", threadID, err)
	}
	return nil
}

func unsubscribe(ctx context.Context, q db.DBTX, threadID, agentID int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM thread_subscriptions WHERE thread_id = ? AND agent_id = ?`,
		threadID, agentID)
	if err != nil {
		return fmt.Errorf("unsubscribing from thread %d: %w", threadID, err)
	}
	return nil
}

func isSubscribed(ctx context.Context, q db.DBTX, threadID, agentID int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM thread_subscriptions WHERE thread_id = ? AND agent_id = ?`,
		threadID, agentID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("checking subscription: %w", err)
	}
	return n > 0, nil
}

func threadSubscribers(ctx context.Context, q db.DBTX, threadID, exclude int64) ([]int64, error) {
	return db.QueryIDs(ctx, q, `SELECT agent_id FROM thread_subscriptions WHERE thread_id = ? AND agent_id <> ? ORDER BY agent_id`,
		threadID, exclude)
}

func boardSubscribers(ctx context.Context, q db.DBTX, boardID, exclude int64) ([]int64, error) {
	return db.QueryIDs(ctx, q, `SELECT DISTINCT s.agent_id FROM thread_subscriptions s JOIN threads t ON t.id = s.thread_id
		WHERE t.blackboard_id = ? AND s.agent_id <> ? ORDER BY s.agent_id`, boardID, exclude)
}

func deleteSpaceSubscriptions(ctx context.Context, q db.DBTX, spaceID, agentID int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM thread_subscriptions WHERE agent_id = ? AND thread_id IN
		(SELECT t.id FROM threads t JOIN blackboards b ON b.id = t.blackboard_id WHERE b.space_id = ?)`, agentID, spaceID)
	if err != nil {
		return fmt.Errorf("deleting thread subscriptions: %w", err)
	}
	return nil
}
