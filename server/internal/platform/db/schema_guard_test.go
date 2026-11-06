package db_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// TestMigrateRefusesNewerSchema: a database migrated by a newer binary (a version this binary
// doesn't know) is refused before any pending migration runs on it, naming both versions.
func TestMigrateRefusesNewerSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	conn := openTemp(t)
	first := file("CREATE TABLE log(id INTEGER PRIMARY KEY, v TEXT);")
	if err := db.Migrate(ctx, conn, fstest.MapFS{"0001_a.up.sql": first}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (3, '2026-12-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	err := db.Migrate(ctx, conn, fstest.MapFS{"0001_a.up.sql": first, "0002_b.up.sql": file("INSERT INTO log(v) VALUES ('b');")})
	want := "database schema version 3 is newer than this binary (latest 2); " +
		"run a newer agent-message-bus or restore the backup taken before the upgrade"
	if err == nil || err.Error() != want {
		t.Fatalf("Migrate on a newer schema: %v, want %q", err, want)
	}
	assertLog(t, conn, nil)
}
