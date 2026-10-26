package spaces

import (
	"context"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
)

// spaceRows counts the rows a space owns directly or by cascade.
func spaceRows(t *testing.T, e env, space string) map[string]int {
	t.Helper()
	q := func(sql string) int { return countRows(t, e.db, sql, space) }
	return map[string]int{
		"spaces":       q(`SELECT COUNT(*) FROM spaces WHERE name = ?`),
		"participants": q(`SELECT COUNT(*) FROM participants p JOIN spaces s ON s.id = p.space_id WHERE s.name = ?`),
		"invites":      q(`SELECT COUNT(*) FROM invites i JOIN spaces s ON s.id = i.space_id WHERE s.name = ?`),
		"tokens":       q(`SELECT COUNT(*) FROM tokens t JOIN spaces s ON s.id = t.space_id WHERE s.name = ?`),
		"inbox":        q(`SELECT COUNT(*) FROM inbox i JOIN spaces s ON s.id = i.space_id WHERE s.name = ?`),
	}
}

func wantGone(t *testing.T, e env, space string) {
	t.Helper()
	for table, n := range spaceRows(t, e, space) {
		if n != 0 {
			t.Errorf("%s rows of space %s = %d, want 0", table, space, n)
		}
	}
}

func seedPendingInvite(t *testing.T, e env, space string) {
	t.Helper()
	_, err := e.db.ExecContext(context.Background(),
		`INSERT INTO invites (hash, space_id, name, expires_at) SELECT 'h', id, 'late', 4102444800 FROM spaces WHERE name = ?`, space)
	if err != nil {
		t.Fatalf("seeding invite: %v", err)
	}
}

func TestLastDepartureDeletesSpace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		depart func(t *testing.T, e env, lead, ops auth.Principal) error
	}{
		{"admin leaves its solo space", func(t *testing.T, e env, _, ops auth.Principal) error {
			mustCreate(t, e, ops, "gamma")
			return e.svc.Leave(context.Background(), ops, "gamma")
		}},
		{"last member of a leaderless space leaves", func(t *testing.T, e env, lead, ops auth.Principal) error {
			mustCreate(t, e, lead, "gamma")
			worker, _ := inviteMember(t, e, lead, "gamma", "worker")
			seedInbox(t, e, auth.Principal{AgentID: worker.AgentID, SpaceID: worker.SpaceID})
			wantRevokeIn(t, e, ops, "gamma", "lead")
			return e.svc.Leave(context.Background(), worker, "gamma")
		}},
		{"admin revokes the lone leader", func(t *testing.T, e env, lead, ops auth.Principal) error {
			mustCreate(t, e, lead, "gamma")
			_, err := e.svc.Revoke(context.Background(), ops, "gamma", "lead")
			return err
		}},
		{"admin revokes the last member of a leaderless space", func(t *testing.T, e env, lead, ops auth.Principal) error {
			mustCreate(t, e, lead, "gamma")
			inviteMember(t, e, lead, "gamma", "worker")
			wantRevokeIn(t, e, ops, "gamma", "lead")
			_, err := e.svc.Revoke(context.Background(), ops, "gamma", "worker")
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hook := &recordingHook{recipient: 1}
			e := newEnv(t, []DepartureHook{hook}, nil)
			lead, ops := leader(t, e, 1, "lead"), opsAdmin(t, e)
			if err := tc.depart(t, e, lead, ops); err != nil {
				t.Fatalf("departure: %v", err)
			}
			wantGone(t, e, "gamma")
		})
	}
}

func wantRevokeIn(t *testing.T, e env, p auth.Principal, space, name string) {
	t.Helper()
	if deleted, err := e.svc.Revoke(context.Background(), p, space, name); err != nil || !deleted {
		t.Fatalf("revoke %s from %s = %v, %v", name, space, deleted, err)
	}
}

func TestPendingInviteDoesNotKeepSpace(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	ops := opsAdmin(t, e)
	mustCreate(t, e, ops, "gamma")
	seedPendingInvite(t, e, "gamma")
	if err := e.svc.Leave(context.Background(), ops, "gamma"); err != nil {
		t.Fatalf("admin leaves: %v", err)
	}
	wantGone(t, e, "gamma")
}

func TestLoneLeaderLeaves(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	lead := leader(t, e, 1, "lead")
	mustCreate(t, e, lead, "solo")
	if err := e.svc.Leave(context.Background(), lead, "solo"); err != nil {
		t.Fatalf("lone leader leaves: %v", err)
	}
	wantGone(t, e, "solo")
}

func TestLastLeaderStillRefusedWhileOthersRemain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t, nil, nil)
	lead, ops := leader(t, e, 1, "lead"), opsAdmin(t, e)
	mustCreate(t, e, lead, "demo")
	mustJoin(t, e, ops, "demo")
	apperrtest.WantKind(t, e.svc.Leave(ctx, lead, "demo"), apperr.Conflict, "last leader cannot leave; delete the space instead")
	if n := spaceRows(t, e, "demo")["spaces"]; n != 1 {
		t.Fatalf("space demo rows = %d, want 1", n)
	}
	if err := e.svc.Leave(ctx, ops, "demo"); err != nil {
		t.Fatalf("admin leaves while the leader remains: %v", err)
	}
	if n := spaceRows(t, e, "demo")["spaces"]; n != 1 {
		t.Fatalf("departure that leaves the leader behind deleted the space")
	}
}
