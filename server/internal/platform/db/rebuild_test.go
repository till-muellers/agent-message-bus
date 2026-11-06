package db_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

const parentChild = `CREATE TABLE parent(id INTEGER PRIMARY KEY);
CREATE TABLE child(id INTEGER PRIMARY KEY, parent_id INTEGER NOT NULL REFERENCES parent(id) ON DELETE CASCADE);
INSERT INTO parent(id) VALUES (1); INSERT INTO child(parent_id) VALUES (1);`

// TestMigrationRebuildsATableTheSQLiteWay: a migration may rebuild a parent table (create,
// copy, drop, rename): the drop does not cascade into its children, the children point at
// the new table afterwards, and foreign keys are enforced again.
func TestMigrationRebuildsATableTheSQLiteWay(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	conn := openTemp(t)
	fsys := fstest.MapFS{
		"0001_a.up.sql": file(parentChild),
		"0002_rebuild.up.sql": file(`CREATE TABLE parent_new(id INTEGER PRIMARY KEY AUTOINCREMENT);
			INSERT INTO parent_new(id) SELECT id FROM parent; DROP TABLE parent;
			ALTER TABLE parent_new RENAME TO parent;`),
	}
	if err := db.Migrate(ctx, conn, fsys); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if n := count(t, conn, `SELECT COUNT(*) FROM child`); n != 1 {
		t.Fatalf("children after the rebuild = %d, want 1", n)
	}
	if n := count(t, conn, `PRAGMA foreign_keys`); n != 1 {
		t.Fatalf("foreign_keys after migrating = %d, want 1", n)
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM parent WHERE id = 1`); err != nil {
		t.Fatalf("deleting the parent: %v", err)
	}
	if n := count(t, conn, `SELECT COUNT(*) FROM child`); n != 0 {
		t.Fatalf("children after deleting their parent = %d, want 0 (cascade)", n)
	}
}

// TestMigrationLeavingADanglingForeignKeyRollsBack: foreign keys are checked before a
// migration commits.
func TestMigrationLeavingADanglingForeignKeyRollsBack(t *testing.T) {
	t.Parallel()
	conn := openTemp(t)
	fsys := fstest.MapFS{
		"0001_a.up.sql":    file(parentChild),
		"0002_bad.up.sql":  file(`INSERT INTO child(parent_id) VALUES (99);`),
		"0003_late.up.sql": file(`INSERT INTO parent(id) VALUES (2);`),
	}
	err := db.Migrate(context.Background(), conn, fsys)
	if err == nil || !strings.Contains(err.Error(), "0002_bad.up.sql") || !strings.Contains(strings.ToLower(err.Error()), "foreign key") {
		t.Fatalf("migrate = %v, want a foreign key error of 0002_bad.up.sql", err)
	}
	if n := count(t, conn, `SELECT COUNT(*) FROM child`); n != 1 {
		t.Fatalf("children = %d, want 1 (0002 rolled back)", n)
	}
	if n := count(t, conn, `SELECT MAX(version) FROM schema_migrations`); n != 1 {
		t.Fatalf("version = %d, want 1", n)
	}
}

func count(t *testing.T, conn *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := conn.QueryRowContext(context.Background(), query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}
