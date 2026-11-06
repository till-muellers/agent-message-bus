package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/migrations"
)

// holdWriteLock makes a second connection take the write lock with BEGIN IMMEDIATE,
// like an operator's sqlite3 session. The returned func releases it; cleanup releases
// it at the latest.
func holdWriteLock(t *testing.T, path string) func() {
	t.Helper()
	ctx := context.Background()
	other, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close() })
	lock, err := other.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lock.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			_, _ = lock.ExecContext(ctx, `ROLLBACK`)
			_ = lock.Close()
		})
	}
	t.Cleanup(release)
	return release
}

func openMigrated(t *testing.T) (*sql.DB, string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "lock.db")
	conn, err := db.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Migrate(ctx, conn, migrations.FS); err != nil {
		t.Fatal(err)
	}
	return conn, path
}

// A read-then-write transaction waits for an external writer (busy_timeout)
// instead of failing with SQLITE_BUSY on the read-to-write lock upgrade.
func TestInTxWaitsForExternalWriteLock(t *testing.T) {
	t.Parallel()
	conn, path := openMigrated(t)
	release := holdWriteLock(t, path)
	released := make(chan struct{})
	time.AfterFunc(300*time.Millisecond, func() {
		close(released)
		release()
	})
	err := db.InTx(context.Background(), conn, func(tx *sql.Tx) error {
		if got := countAgents(t, tx); got != 0 {
			t.Errorf("agents = %d, want 0", got)
		}
		_, err := tx.Exec(`INSERT INTO agents (name, kind, created_at) VALUES ('lead', 'leader', 'x')`)
		return err
	})
	if err != nil {
		t.Fatalf("InTx during external write lock: %v", err)
	}
	select {
	case <-released:
	default:
		t.Fatal("InTx wrote while the external write lock was held")
	}
	if got := countAgents(t, conn); got != 1 {
		t.Fatalf("agents = %d, want 1", got)
	}
}

// Plain reads outside InTx are WAL readers: they do not wait for an external writer.
func TestReadDoesNotWaitForExternalWriteLock(t *testing.T) {
	t.Parallel()
	conn, path := openMigrated(t)
	var mode string
	if err := conn.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal_mode = %q (%v), want wal", mode, err)
	}
	release := holdWriteLock(t, path)
	countAgents(t, conn) // a waiting read would fail with SQLITE_BUSY after busy_timeout
	release()
}
