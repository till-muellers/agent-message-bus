package migrations_test

import (
	"context"
	"database/sql"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/migrations"
)

const autoincrement = "0005_autoincrement_ids"

// seedEveryTable fills every table, with space 2 holding one row of each space-scoped table.
const seedEveryTable = `
INSERT INTO agents (id, name, kind, created_at) VALUES (1, 'lead', 'leader', 't'), (2, 'worker', 'member', 't');
INSERT INTO spaces (id, name, created_by, created_at) VALUES (1, 'keep', 1, 't'), (2, 'demo', 1, 't');
INSERT INTO tokens (hash, agent_id, space_id, created_at) VALUES ('h-lead', 1, NULL, 't'), ('h-worker', 2, 2, 't');
INSERT INTO participants (space_id, agent_id, role, joined_at) VALUES (1, 1, 'leader', 't'), (2, 1, 'leader', 't'), (2, 2, 'member', 't');
INSERT INTO topics (id, space_id, name, created_by, created_at) VALUES (5, 2, 'all', 1, 't');
INSERT INTO topic_subscriptions (topic_id, agent_id) VALUES (5, 2);
INSERT INTO blackboards (id, space_id, name, owner_id, created_at) VALUES (7, 2, 'b', 1, 't');
INSERT INTO threads (id, blackboard_id, title, created_by, created_at) VALUES (3, 7, 'one', 1, 't'), (4, 7, 'two', 2, 't');
INSERT INTO posts (thread_id, author_id, body, created_at) VALUES (3, 2, 'hi', 't');
INSERT INTO thread_subscriptions (thread_id, agent_id) VALUES (3, 2);
INSERT INTO inbox (agent_id, space_id, type, from_name, created_at) VALUES (2, 2, 'direct_message', 'lead', 't');
INSERT INTO invites (hash, space_id, name, expires_at, invited_by, id) VALUES ('i', 2, 'late', 9999999999, 'lead', 'abc');`

// TestAutoincrementIDsMigration: spaces and threads keep every row, id and foreign key
// through the rebuild, and never reuse the id of a deleted row afterwards.
func TestAutoincrementIDsMigration(t *testing.T) {
	t.Parallel()
	conn := seededBefore(t, autoincrement)
	before := rowCounts(t, conn)
	migrateWith(t, conn, migrations.FS)
	if got := rowCounts(t, conn); !maps.Equal(got, before) {
		t.Fatalf("rows after the migration = %v, want %v", got, before)
	}
	wantAutoincrement(t, conn, true)
	if ids := queryColumn[int](t, conn, `SELECT id FROM threads ORDER BY id`); !slices.Equal(ids, []int{3, 4}) {
		t.Fatalf("thread ids = %v, want [3 4]", ids)
	}
	mustExec(t, conn, `DELETE FROM spaces WHERE id = 2`)
	wantCascaded(t, conn)
	mustExec(t, conn, `INSERT INTO spaces (name, created_by, created_at) VALUES ('next', 1, 't')`)
	mustExec(t, conn, `INSERT INTO blackboards (id, space_id, name, owner_id, created_at) VALUES (8, 1, 'b', 1, 't')`)
	mustExec(t, conn, `INSERT INTO threads (blackboard_id, title, created_by, created_at) VALUES (8, 'next', 1, 't')`)
	if ids := queryColumn[int](t, conn, `SELECT MAX(id) FROM spaces UNION ALL SELECT MAX(id) FROM threads`); !slices.Equal(ids, []int{3, 5}) {
		t.Fatalf("new space and thread ids = %v, want [3 5] (2 and 4 were deleted)", ids)
	}
}

// TestAutoincrementIDsMigrationReverts: the down file, applied the way the runner applies
// migrations, restores the plain INTEGER PRIMARY KEY and keeps every row.
func TestAutoincrementIDsMigrationReverts(t *testing.T) {
	t.Parallel()
	conn := seededBefore(t, autoincrement)
	before := rowCounts(t, conn)
	withRevert := through(t, autoincrement+".up.sql")
	withRevert["9999_revert.up.sql"] = &fstest.MapFile{Data: readMigration(t, autoincrement+".down.sql")}
	migrateWith(t, conn, withRevert)
	if got := rowCounts(t, conn); !maps.Equal(got, before) {
		t.Fatalf("rows after reverting = %v, want %v", got, before)
	}
	wantAutoincrement(t, conn, false)
	if n := queryColumn[int](t, conn, `SELECT COUNT(*) FROM sqlite_sequence WHERE name IN ('spaces', 'threads')`); n[0] != 0 {
		t.Fatalf("sqlite_sequence rows of spaces and threads = %d, want 0", n[0])
	}
	mustExec(t, conn, `DELETE FROM spaces WHERE id = 2`)
	wantCascaded(t, conn)
}

// seededBefore returns a database migrated up to the migration named name, with seedEveryTable.
func seededBefore(t *testing.T, name string) *sql.DB {
	t.Helper()
	conn, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	migrateWith(t, conn, before(t, "_"+strings.SplitN(name, "_", 2)[1]+".up.sql"))
	mustExec(t, conn, seedEveryTable)
	return conn
}

// through returns the up migrations of migrations.FS up to and including name.
func through(t *testing.T, name string) fstest.MapFS {
	t.Helper()
	out := before(t, strings.TrimPrefix(name, name[:4]))
	out[name] = &fstest.MapFile{Data: readMigration(t, name)}
	return out
}

func readMigration(t *testing.T, name string) []byte {
	t.Helper()
	body, err := fs.ReadFile(migrations.FS, name)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return body
}

func migrateWith(t *testing.T, conn *sql.DB, fsys fs.FS) {
	t.Helper()
	if err := db.Migrate(context.Background(), conn, fsys); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

// rowCounts returns the row count of every table.
func rowCounts(t *testing.T, conn *sql.DB) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, table := range wantTables {
		out[table] = queryColumn[int](t, conn, `SELECT COUNT(*) FROM `+table)[0]
	}
	return out
}

// wantAutoincrement asserts whether spaces and threads are declared AUTOINCREMENT.
func wantAutoincrement(t *testing.T, conn *sql.DB, want bool) {
	t.Helper()
	for _, table := range []string{"spaces", "threads"} {
		ddl := queryColumn[string](t, conn, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, table)
		if len(ddl) != 1 || strings.Contains(ddl[0], "AUTOINCREMENT") != want {
			t.Fatalf("%s ddl = %q, want AUTOINCREMENT %v", table, ddl, want)
		}
	}
}

// wantCascaded asserts that deleting space 2 removed every row that hung off it.
func wantCascaded(t *testing.T, conn *sql.DB) {
	t.Helper()
	want := map[string]int{"agents": 2, "spaces": 1, "tokens": 1, "participants": 1, "topics": 0,
		"topic_subscriptions": 0, "blackboards": 0, "threads": 0, "posts": 0, "thread_subscriptions": 0,
		"inbox": 0, "invites": 0}
	if got := rowCounts(t, conn); !maps.Equal(got, want) {
		t.Fatalf("rows after deleting space 2 = %v, want %v", got, want)
	}
}
