package spaces

import (
	"context"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
)

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
			wantSpace(t, e, "gamma", false)
			if n, err := e.inbox.Count(context.Background(), lead); err != nil || n != 0 {
				t.Fatalf("departure hook entries for lead = %d, %v; want dropped", n, err)
			}
		})
	}
}

func wantRevokeIn(t *testing.T, e env, p auth.Principal, space, name string) {
	t.Helper()
	if deleted, err := e.svc.Revoke(context.Background(), p, space, name); err != nil || !deleted {
		t.Fatalf("revoke %s from %s = %v, %v", name, space, deleted, err)
	}
}

func TestLoneLeaderLeaves(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	lead := leader(t, e, 1, "lead")
	mustCreate(t, e, lead, "solo")
	if err := e.svc.Leave(context.Background(), lead, "solo"); err != nil {
		t.Fatalf("lone leader leaves: %v", err)
	}
	wantSpace(t, e, "solo", false)
}

func TestLastLeaderStillRefusedWhileOthersRemain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t, nil, nil)
	lead, ops := leader(t, e, 1, "lead"), opsAdmin(t, e)
	mustCreate(t, e, lead, "demo")
	mustJoin(t, e, ops, "demo")
	apperrtest.WantKind(t, e.svc.Leave(ctx, lead, "demo"), apperr.Conflict, "last leader cannot leave; delete the space instead")
	wantSpace(t, e, "demo", true)
	if err := e.svc.Leave(ctx, ops, "demo"); err != nil {
		t.Fatalf("admin leaves while the leader remains: %v", err)
	}
	wantSpace(t, e, "demo", true)
}
