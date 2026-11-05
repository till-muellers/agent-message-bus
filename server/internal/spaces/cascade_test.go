package spaces

import (
	"context"
	"testing"
)

// TestSpaceDeletionCascades is the one deliberate schema-level test of space deletion: it
// counts, by space id, the rows other features keep for a space (an open invite, a member
// token, an inbox entry), which only the ON DELETE CASCADE of their space_id removes when
// an admin deletes the space. Every other test asserts through the services.
func TestSpaceDeletionCascades(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	lead, ops := leader(t, e, 1, "lead"), opsAdmin(t, e)
	mustCreate(t, e, lead, "gamma")
	worker, _ := inviteMember(t, e, lead, "gamma", "worker")
	seedInbox(t, e, worker)
	seedPendingInvite(t, e, worker.SpaceID)
	if err := e.svc.Delete(context.Background(), ops, "gamma"); err != nil {
		t.Fatalf("admin deletes gamma: %v", err)
	}
	for table, n := range spaceRows(t, e, worker.SpaceID) {
		if n != 0 {
			t.Errorf("%s rows of space gamma = %d, want 0", table, n)
		}
	}
}

// spaceRows counts the rows of every table that references spaceID.
func spaceRows(t *testing.T, e env, spaceID int64) map[string]int {
	t.Helper()
	count := func(query string) int {
		var n int
		if err := e.db.QueryRowContext(context.Background(), query, spaceID).Scan(&n); err != nil {
			t.Fatalf("count %q: %v", query, err)
		}
		return n
	}
	return map[string]int{
		"spaces":       count(`SELECT COUNT(*) FROM spaces WHERE id = ?`),
		"participants": count(`SELECT COUNT(*) FROM participants WHERE space_id = ?`),
		"invites":      count(`SELECT COUNT(*) FROM invites WHERE space_id = ?`),
		"tokens":       count(`SELECT COUNT(*) FROM tokens WHERE space_id = ?`),
		"inbox":        count(`SELECT COUNT(*) FROM inbox WHERE space_id = ?`),
	}
}

// seedPendingInvite inserts an open invite row for spaceID; spaces cannot use the invites
// service (invites imports spaces), so the schema-level test writes the row itself.
func seedPendingInvite(t *testing.T, e env, spaceID int64) {
	t.Helper()
	_, err := e.db.ExecContext(context.Background(),
		`INSERT INTO invites (hash, space_id, name, expires_at) VALUES ('h', ?, 'late', 4102444800)`, spaceID)
	if err != nil {
		t.Fatalf("seeding invite: %v", err)
	}
}
