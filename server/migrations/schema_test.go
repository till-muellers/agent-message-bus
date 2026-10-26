package migrations_test

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
	"github.com/till-muellers/agent-message-bus/server/migrations"
)

const schemaFile = "0001_schema.up.sql"

var wantTables = []string{
	"agents", "blackboards", "inbox", "invites", "participants", "posts",
	"spaces", "thread_subscriptions", "threads", "tokens", "topic_subscriptions", "topics",
}

func TestSchemaReset(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		check func(t *testing.T, conn *sql.DB)
	}{
		{"fresh database holds exactly version 1", func(t *testing.T, conn *sql.DB) {
			assertVersions(t, conn, []int{1})
		}},
		{"fresh database has the twelve tables", func(t *testing.T, conn *sql.DB) {
			if got := tables(t, conn); !slices.Equal(got, wantTables) {
				t.Fatalf("tables = %v, want %v", got, wantTables)
			}
		}},
		{"agents accept kind admin", func(t *testing.T, conn *sql.DB) {
			mustExec(t, conn, insertAgent, "admin")
		}},
		{"agents reject kind owner", func(t *testing.T, conn *sql.DB) {
			assertCheckError(t, conn, insertAgent, "owner")
		}},
		{"participants accept role admin", func(t *testing.T, conn *sql.DB) {
			seedSpace(t, conn)
			mustExec(t, conn, insertParticipant, "admin")
		}},
		{"participants reject role owner", func(t *testing.T, conn *sql.DB) {
			seedSpace(t, conn)
			assertCheckError(t, conn, insertParticipant, "owner")
		}},
		{"second migrate applies nothing", func(t *testing.T, conn *sql.DB) {
			if err := db.Migrate(context.Background(), conn, migrations.FS); err != nil {
				t.Fatalf("second migrate: %v", err)
			}
			assertVersions(t, conn, []int{1})
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.check(t, dbtest.Open(t))
		})
	}
}

func TestLaterMigrationAppliesOnTop(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fsys := withExample(t)
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	for range 2 {
		if err := db.Migrate(ctx, conn, fsys); err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}
	assertVersions(t, conn, []int{1, 2})
	var n int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM example`).Scan(&n); err != nil {
		t.Fatalf("count example: %v", err)
	}
	if n != 1 {
		t.Fatalf("example rows = %d, want 1 (applied once)", n)
	}
}

const (
	insertAgent       = `INSERT INTO agents(name, kind, created_at) VALUES ('a', ?, 't')`
	insertParticipant = `INSERT INTO participants(space_id, agent_id, role, joined_at) VALUES (1, 1, ?, 't')`
)

func withExample(t *testing.T) fstest.MapFS {
	t.Helper()
	body, err := fs.ReadFile(migrations.FS, schemaFile)
	if err != nil {
		t.Fatalf("reading %s: %v", schemaFile, err)
	}
	return fstest.MapFS{
		schemaFile: {Data: body},
		"0002_example.up.sql": {Data: []byte(
			"CREATE TABLE example(id INTEGER PRIMARY KEY); INSERT INTO example DEFAULT VALUES;")},
	}
}

func seedSpace(t *testing.T, conn *sql.DB) {
	t.Helper()
	mustExec(t, conn, `INSERT INTO agents(id, name, kind, created_at) VALUES (1, 'l', 'leader', 't')`)
	mustExec(t, conn, `INSERT INTO spaces(id, name, created_by, created_at) VALUES (1, 's', 1, 't')`)
}

func mustExec(t *testing.T, conn *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := conn.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q %v: %v", query, args, err)
	}
}

func assertCheckError(t *testing.T, conn *sql.DB, query string, value string) {
	t.Helper()
	_, err := conn.ExecContext(context.Background(), query, value)
	if err == nil || !strings.Contains(err.Error(), "CHECK constraint failed") {
		t.Fatalf("insert %q: err = %v, want CHECK constraint failed", value, err)
	}
}

func assertVersions(t *testing.T, conn *sql.DB, want []int) {
	t.Helper()
	got := queryColumn[int](t, conn, `SELECT version FROM schema_migrations ORDER BY version`)
	if !slices.Equal(got, want) {
		t.Fatalf("schema_migrations = %v, want %v", got, want)
	}
}

func tables(t *testing.T, conn *sql.DB) []string {
	t.Helper()
	return queryColumn[string](t, conn, `SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name != 'schema_migrations' ORDER BY name`)
}

func queryColumn[T any](t *testing.T, conn *sql.DB, query string) []T {
	t.Helper()
	rows, err := conn.QueryContext(context.Background(), query)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []T
	for rows.Next() {
		var v T
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}
