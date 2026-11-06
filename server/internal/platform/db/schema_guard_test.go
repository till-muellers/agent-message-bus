package db_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// TestMigrateRefusesNewerSchema: a database migrated by a newer binary (a version this binary
// doesn't know) is refused before anything runs on it, naming both versions.
func TestMigrateRefusesNewerSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	conn := openTemp(t)
	fsys := fstest.MapFS{"0001_a.up.sql": file("CREATE TABLE log(id INTEGER PRIMARY KEY, v TEXT);")}
	if err := db.Migrate(ctx, conn, fsys); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (2, '2026-12-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	err := db.Migrate(ctx, conn, fsys)
	want := "database schema version 2 is newer than this binary (latest 1); " +
		"run a newer agent-message-bus or restore the backup taken before the upgrade"
	if err == nil || err.Error() != want {
		t.Fatalf("Migrate on a newer schema: %v, want %q", err, want)
	}
}
