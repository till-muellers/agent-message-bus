package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

func openTemp(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func file(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

func TestMigrate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		fsys    fstest.MapFS
		wantErr string
		want    []string // rows of table log, in order
		version int
	}{
		{
			name: "applies in numeric order and ignores others",
			fsys: fstest.MapFS{
				"0002_b.up.sql":   file("INSERT INTO log(v) VALUES ('b');"),
				"0001_a.up.sql":   file("CREATE TABLE log(id INTEGER PRIMARY KEY, v TEXT); INSERT INTO log(v) VALUES ('a');"),
				"0001_a.down.sql": file("DROP TABLE log;"),
				"README.md":       file("not sql"),
				"1_bad.up.sql":    file("garbage"),
			},
			want:    []string{"a", "b"},
			version: 2,
		},
		{
			name: "failing sql rolls back",
			fsys: fstest.MapFS{
				"0001_a.up.sql": file("CREATE TABLE log(id INTEGER PRIMARY KEY, v TEXT); INSERT INTO log(v) VALUES ('a');"),
				"0002_b.up.sql": file("INSERT INTO log(v) VALUES ('b'); BOGUS;"),
			},
			wantErr: "0002_b.up.sql",
			want:    []string{"a"},
			version: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			conn := openTemp(t)
			for range 2 { // second run must be idempotent
				err := db.Migrate(ctx, conn, tc.fsys)
				if tc.wantErr == "" && err != nil {
					t.Fatalf("migrate: %v", err)
				}
				if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
			}
			assertLog(t, conn, tc.want)
			var v int
			if err := conn.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations").Scan(&v); err != nil {
				t.Fatalf("version: %v", err)
			}
			if v != tc.version {
				t.Fatalf("version = %d, want %d", v, tc.version)
			}
		})
	}
}

func assertLog(t *testing.T, conn *sql.DB, want []string) {
	t.Helper()
	rows, err := conn.Query("SELECT v FROM log ORDER BY id")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, s)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("log = %v, want %v", got, want)
	}
}

func TestRealMigrations(t *testing.T) {
	t.Parallel()
	conn := dbtest.Open(t)
	var fk int
	if err := conn.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys = %d, err %v", fk, err)
	}
	var name string
	err := conn.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='inbox'").Scan(&name)
	if err != nil {
		t.Fatalf("inbox table: %v", err)
	}
}
