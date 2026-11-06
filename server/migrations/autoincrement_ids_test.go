package migrations_test

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/migrations"
)

// autoincrement is the name suffix of the migration under test; its number is set at merge time.
const autoincrement = "_autoincrement_ids"

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
	seeded, shape := rowCounts(t, conn), tableShapes(t, conn)
	migrateWith(t, conn, migrations.FS)
	if got := rowCounts(t, conn); !maps.Equal(got, seeded) {
		t.Fatalf("rows after the migration = %v, want %v", got, seeded)
	}
	wantShapes(t, conn, shape)
	if ids := queryColumn[int](t, conn, `SELECT id FROM threads ORDER BY id`); !slices.Equal(ids, []int{3, 4}) {
		t.Fatalf("thread ids = %v, want [3 4]", ids)
	}
	mustExec(t, conn, `DELETE FROM spaces WHERE id = 2`)
	wantCascaded(t, conn)
	mustExec(t, conn, `INSERT INTO spaces (name, created_by, created_at) VALUES ('next', 1, 't')`)
	mustExec(t, conn, `INSERT INTO blackboards (id, space_id, name, owner_id, created_at) VALUES (8, 1, 'b', 1, 't')`)
	mustExec(t, conn, `INSERT INTO threads (blackboard_id, title, created_by, created_at) VALUES (8, 'next', 1, 't')`)
	space := queryColumn[int](t, conn, `SELECT id FROM spaces WHERE name = 'next'`)
	thread := queryColumn[int](t, conn, `SELECT id FROM threads WHERE title = 'next'`)
	if space[0] != 3 || thread[0] != 5 {
		t.Fatalf("new space id %v, thread id %v; want 3 and 5 (2 and 4 were deleted)", space, thread)
	}
}

// TestAutoincrementIDsMigrationReverts: the down file, applied the way the runner applies
// migrations, restores the plain INTEGER PRIMARY KEY and keeps every row.
func TestAutoincrementIDsMigrationReverts(t *testing.T) {
	t.Parallel()
	conn := seededBefore(t, autoincrement)
	seeded, shape := rowCounts(t, conn), tableShapes(t, conn)
	up := migrationFile(t, autoincrement+".up.sql")
	withRevert := before(t, autoincrement+".up.sql")
	withRevert[up] = &fstest.MapFile{Data: readMigration(t, up)}
	withRevert["9999_revert.up.sql"] = &fstest.MapFile{Data: readMigration(t, migrationFile(t, autoincrement+".down.sql"))}
	migrateWith(t, conn, withRevert)
	if got := rowCounts(t, conn); !maps.Equal(got, seeded) {
		t.Fatalf("rows after reverting = %v, want %v", got, seeded)
	}
	wantShapes(t, conn, shape)
	if n := queryColumn[int](t, conn, `SELECT COUNT(*) FROM sqlite_sequence WHERE name IN ('spaces', 'threads')`); n[0] != 0 {
		t.Fatalf("sqlite_sequence rows of spaces and threads = %d, want 0", n[0])
	}
	mustExec(t, conn, `DELETE FROM spaces WHERE id = 2`)
	wantCascaded(t, conn)
	mustExec(t, conn, `INSERT INTO spaces (name, created_by, created_at) VALUES ('next', 1, 't')`)
	if id := queryColumn[int](t, conn, `SELECT id FROM spaces WHERE name = 'next'`); id[0] != 2 {
		t.Fatalf("new space id = %v, want 2 again (plain INTEGER PRIMARY KEY)", id)
	}
}

// seededBefore returns a database migrated up to the migration whose name ends in
// suffix+".up.sql", holding seedEveryTable.
func seededBefore(t *testing.T, suffix string) *sql.DB {
	t.Helper()
	conn, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	migrateWith(t, conn, before(t, suffix+".up.sql"))
	mustExec(t, conn, seedEveryTable)
	return conn
}

// migrationFile returns the name of the one migration file ending in suffix.
func migrationFile(t *testing.T, suffix string) string {
	t.Helper()
	names, err := fs.Glob(migrations.FS, "*"+suffix)
	if err != nil || len(names) != 1 {
		t.Fatalf("migrations ending in %s = %v, %v; want one", suffix, names, err)
	}
	return names[0]
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

// tableShapes returns the columns, foreign keys and indexes of spaces and threads.
func tableShapes(t *testing.T, conn *sql.DB) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, table := range []string{"spaces", "threads"} {
		out[table] = slices.Concat(
			queryRows(t, conn, `SELECT * FROM pragma_table_xinfo(?)`, table),
			queryRows(t, conn, `SELECT * FROM pragma_foreign_key_list(?)`, table),
			queryRows(t, conn, `SELECT l.name, l."unique", l.origin, l.partial, x.*
				FROM pragma_index_list(?) AS l, pragma_index_xinfo(l.name) AS x ORDER BY l.name, x.seqno`, table))
	}
	return out
}

// wantShapes asserts that spaces and threads still have the shape tableShapes recorded.
func wantShapes(t *testing.T, conn *sql.DB, want map[string][]string) {
	t.Helper()
	if got := tableShapes(t, conn); !maps.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("table shapes = %q, want %q", got, want)
	}
}

// queryRows returns every row of query, each formatted as its space-separated values.
func queryRows(t *testing.T, conn *sql.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := conn.QueryContext(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("columns: %v", err)
	}
	var out []string
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, fmt.Sprint(values))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
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
