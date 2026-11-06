package db

import (
	"context"
	"database/sql"
	"fmt"
)

// ensureKnownSchema refuses a database that a newer binary migrated: running on a schema it
// doesn't know, this binary would corrupt data (an older INSERT missing a newer column).
func ensureKnownSchema(ctx context.Context, db *sql.DB, known []migration) error {
	var current int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	latest := 0
	if len(known) > 0 {
		latest = known[len(known)-1].version
	}
	if current > latest {
		return fmt.Errorf("database schema version %d is newer than this binary (latest %d); "+
			"run a newer agent-message-bus or restore the backup taken before the upgrade", current, latest)
	}
	return nil
}
