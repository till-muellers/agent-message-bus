package dbtest

import (
	"context"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// SeedTime is the created_at/joined_at value used by the seed helpers.
const SeedTime = "2026-01-01T00:00:00Z"

// Exec runs q against conn and fails the test on error.
func Exec(t *testing.T, conn db.DBTX, q string, args ...any) {
	t.Helper()
	if _, err := conn.ExecContext(context.Background(), q, args...); err != nil {
		t.Fatalf("dbtest exec %q: %v", q, err)
	}
}

// SeedAgent inserts agent id with name and kind ("leader" or "member").
func SeedAgent(t *testing.T, conn db.DBTX, id int64, name, kind string) {
	t.Helper()
	Exec(t, conn, `INSERT INTO agents (id, name, kind, created_at) VALUES (?, ?, ?, ?)`, id, name, kind, SeedTime)
}

// SeedSpace inserts space id with name, created by agent createdBy.
func SeedSpace(t *testing.T, conn db.DBTX, id int64, name string, createdBy int64) {
	t.Helper()
	Exec(t, conn, `INSERT INTO spaces (id, name, created_by, created_at) VALUES (?, ?, ?, ?)`, id, name, createdBy, SeedTime)
}

// SeedParticipant adds agent to space with role ("leader" or "member").
func SeedParticipant(t *testing.T, conn db.DBTX, spaceID, agentID int64, role string) {
	t.Helper()
	Exec(t, conn, `INSERT INTO participants (space_id, agent_id, role, joined_at) VALUES (?, ?, ?, ?)`, spaceID, agentID, role, SeedTime)
}
