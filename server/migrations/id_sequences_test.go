package migrations_test

import (
	"testing"

	"github.com/till-muellers/agent-message-bus/server/migrations"
)

// removedThread is thread 9 of blackboard 7, removed while member 2 still holds its
// thread_removed entry.
const removedThread = `
INSERT INTO threads (id, blackboard_id, title, created_by, created_at) VALUES (9, 7, 'gone', 1, 't');
DELETE FROM threads WHERE id = 9;
INSERT INTO inbox (agent_id, space_id, type, from_name, blackboard, thread_id, thread_title, created_at)
  VALUES (2, 2, 'thread_removed', 'lead', 'b', 9, 'gone', 't');`

// TestIDSequencesMigration: no thread id an inbox entry still names is handed out again,
// and a sequence already past every such id is never lowered.
func TestIDSequencesMigration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, before, seed string
		want               int
	}{
		{"thread removed before 0005_autoincrement_ids", autoincrement, removedThread, 10},
		{"sequence already past the removed thread", "_id_sequences", removedThread + `
INSERT INTO threads (id, blackboard_id, title, created_by, created_at) VALUES (12, 7, 'later', 1, 't');
DELETE FROM threads WHERE id = 12;`, 13},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			conn := seededBefore(t, tc.before)
			mustExec(t, conn, tc.seed)
			migrateWith(t, conn, migrations.FS)
			mustExec(t, conn, `INSERT INTO threads (blackboard_id, title, created_by, created_at) VALUES (7, 'next', 1, 't')`)
			if got := queryColumn[int](t, conn, `SELECT id FROM threads WHERE title = 'next'`); got[0] != tc.want {
				t.Fatalf("new thread id = %d, want %d", got[0], tc.want)
			}
		})
	}
}
