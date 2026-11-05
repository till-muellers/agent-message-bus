package spaces

import (
	"context"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
)

// wantGone asserts through the service that space no longer exists. The rows other
// features keep for it are covered by the schema-level test in cascade_test.go.
func wantGone(t *testing.T, e env, space string) {
	t.Helper()
	_, err := e.svc.Participants(context.Background(), auth.Principal{Kind: auth.Admin}, space)
	apperrtest.WantKind(t, err, apperr.NotFound, "space "+space+" not found")
}

// wantExists asserts through the service that space still exists.
func wantExists(t *testing.T, e env, space string) {
	t.Helper()
	if _, err := e.svc.Participants(context.Background(), auth.Principal{Kind: auth.Admin}, space); err != nil {
		t.Fatalf("space %s: %v", space, err)
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
	wantExists(t, e, "demo")
	if err := e.svc.Leave(ctx, ops, "demo"); err != nil {
		t.Fatalf("admin leaves while the leader remains: %v", err)
	}
	wantExists(t, e, "demo")
}
