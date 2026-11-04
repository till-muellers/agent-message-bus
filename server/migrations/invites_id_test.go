package migrations_test

import (
	"context"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/migrations"
)

// TestInvitesIDMigrationBackfills: invites pending before the invites_id migration get
// distinct opaque ids.
func TestInvitesIDMigrationBackfills(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Migrate(ctx, conn, before(t, "_invites_id.up.sql")); err != nil {
		t.Fatalf("migrate before invites_id: %v", err)
	}
	seedSpace(t, conn)
	for _, hash := range []string{"h1", "h2"} {
		mustExec(t, conn, `INSERT INTO invites(hash, space_id, name, expires_at) VALUES (?, 1, 'w', 9999999999)`, hash)
	}
	if err := db.Migrate(ctx, conn, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ids := queryColumn[string](t, conn, `SELECT id FROM invites ORDER BY hash`)
	hex16 := regexp.MustCompile(`^[0-9a-f]{16}$`)
	if len(ids) != 2 || ids[0] == ids[1] || !hex16.MatchString(ids[0]) || !hex16.MatchString(ids[1]) {
		t.Fatalf("backfilled ids = %q, want two distinct 16-char hex ids", ids)
	}
}

// before returns the up migrations of migrations.FS that sort before the one ending in suffix.
func before(t *testing.T, suffix string) fstest.MapFS {
	t.Helper()
	names, err := fs.Glob(migrations.FS, "*.up.sql")
	if err != nil {
		t.Fatalf("listing migrations: %v", err)
	}
	out := fstest.MapFS{}
	for _, name := range names {
		if strings.HasSuffix(name, suffix) {
			return out
		}
		body, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		out[name] = &fstest.MapFile{Data: body}
	}
	t.Fatalf("no migration ending in %s", suffix)
	return nil
}
