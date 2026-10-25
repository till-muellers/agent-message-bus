// Package dbtest provides migrated temporary SQLite databases for tests.
package dbtest

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/migrations"
)

// Open returns a migrated SQLite database in t.TempDir, closed on cleanup.
func Open(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("dbtest: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Migrate(ctx, conn, migrations.FS); err != nil {
		t.Fatalf("dbtest: %v", err)
	}
	return conn
}
