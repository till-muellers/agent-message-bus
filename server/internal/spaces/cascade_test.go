package spaces

import (
	"context"
	"testing"
)

// TestSpaceDeletionCascades is the one deliberate schema-level test of space deletion: it
// counts the rows other features keep for a space (invites, member tokens, inbox), which
// disappear only through the ON DELETE CASCADE of their space_id. Every other test asserts
// through the services.
func TestSpaceDeletionCascades(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	lead, ops := leader(t, e, 1, "lead"), opsAdmin(t, e)
	mustCreate(t, e, lead, "gamma")
	worker, _ := inviteMember(t, e, lead, "gamma", "worker")
	seedInbox(t, e, worker)
	seedPendingInvite(t, e, "gamma")
	wantRevokeIn(t, e, ops, "gamma", "lead")
	if err := e.svc.Leave(context.Background(), worker, "gamma"); err != nil {
		t.Fatalf("last member leaves: %v", err)
	}
	for table, n := range spaceRows(t, e, "gamma") {
		if n != 0 {
			t.Errorf("%s rows of space gamma = %d, want 0", table, n)
		}
	}
}

// spaceRows counts the rows a space owns directly or by cascade.
func spaceRows(t *testing.T, e env, space string) map[string]int {
	t.Helper()
	count := func(query string) int {
		var n int
		if err := e.db.QueryRowContext(context.Background(), query, space).Scan(&n); err != nil {
			t.Fatalf("count %q: %v", query, err)
		}
		return n
	}
	return map[string]int{
		"spaces":       count(`SELECT COUNT(*) FROM spaces WHERE name = ?`),
		"participants": count(`SELECT COUNT(*) FROM participants p JOIN spaces s ON s.id = p.space_id WHERE s.name = ?`),
		"invites":      count(`SELECT COUNT(*) FROM invites i JOIN spaces s ON s.id = i.space_id WHERE s.name = ?`),
		"tokens":       count(`SELECT COUNT(*) FROM tokens t JOIN spaces s ON s.id = t.space_id WHERE s.name = ?`),
		"inbox":        count(`SELECT COUNT(*) FROM inbox i JOIN spaces s ON s.id = i.space_id WHERE s.name = ?`),
	}
}

// seedPendingInvite inserts an open invite row for space; spaces cannot use the invites
// service (invites imports spaces), so the schema-level test writes the row itself.
func seedPendingInvite(t *testing.T, e env, space string) {
	t.Helper()
	_, err := e.db.ExecContext(context.Background(),
		`INSERT INTO invites (hash, space_id, name, expires_at) SELECT 'h', id, 'late', 4102444800 FROM spaces WHERE name = ?`, space)
	if err != nil {
		t.Fatalf("seeding invite: %v", err)
	}
}
