package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

func TestOpenCreatesParentDir(t *testing.T) {
	t.Parallel()
	conn, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "a", "b", "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_ = conn.Close()
}
