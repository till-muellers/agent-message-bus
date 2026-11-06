package spaces

import (
	"context"
	"maps"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
)

// TestRevokedAdminActsNowhere: a call authenticated before bus_revoke_admin committed and
// run after it fails unauthenticated and leaves nothing behind: no participation, no
// orphan space, no new member, no removal.
func TestRevokedAdminActsNowhere(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t, nil, nil)
	root := bootstrapRoot(t, e)
	lead := leader(t, e, 50, "lead")
	mustCreate(t, e, lead, "demo")
	inviteMember(t, e, lead, "demo", "worker")
	mintedAdmin(t, e, root, "web")
	_, ops := mintedAdmin(t, e, root, "ops")
	if _, err := e.svc.RevokeAdmin(ctx, root, "ops"); err != nil {
		t.Fatalf("RevokeAdmin(ops): %v", err)
	}
	staleCalls := []struct {
		name string
		call func() error
	}{
		{"create", func() error { return e.svc.Create(ctx, ops, "orphan") }},
		{"join", func() error { return e.svc.Join(ctx, ops, "demo") }},
		{"invite", func() error { _, err := invite(ctx, e, ops, "demo", "late"); return err }},
		{"revoke member", func() error { _, err := e.svc.Revoke(ctx, ops, "demo", "worker"); return err }},
		{"revoke admin", func() error { _, err := e.svc.RevokeAdmin(ctx, ops, "web"); return err }},
		{"delete", func() error { return e.svc.Delete(ctx, ops, "demo") }},
	}
	for _, c := range staleCalls {
		t.Run(c.name, func(t *testing.T) {
			apperrtest.WantKind(t, c.call(), apperr.Unauthenticated, "invalid token")
		})
	}
	wantSpace(t, e, "orphan", false)
	wantParticipants(t, e, "demo", []Participant{{"lead", RoleLeader, false}, {"worker", RoleMember, false}})
	wantAdmins(t, e, root, map[string]int{"admin": 0, "ops": 0, "web": 1})
}

// TestReinvitedMemberIgnoresOldToken: a call of a revoked member token that runs after the
// same name was invited again does not act as the new membership.
func TestReinvitedMemberIgnoresOldToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t, nil, nil)
	lead := leader(t, e, 50, "lead")
	mustCreate(t, e, lead, "alpha")
	old, _ := inviteMember(t, e, lead, "alpha", "worker")
	wantRevoke(t, e, lead, "worker", true)
	inviteMember(t, e, lead, "alpha", "worker")
	apperrtest.WantKind(t, e.svc.Leave(ctx, old, "alpha"), apperr.Unauthenticated, "invalid token")
	wantParticipants(t, e, "alpha", []Participant{{"lead", RoleLeader, false}, {"worker", RoleMember, false}})
}

// TestRotatedBootstrapTokenActsNowhere: a call of the bootstrap admin authenticated with the
// instance token before a rotation and run after it fails unauthenticated.
func TestRotatedBootstrapTokenActsNowhere(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t, nil, nil)
	root := bootstrapRoot(t, e)
	if err := e.auth.SetAdminToken("rotated-admin-secret-rotated-admin-secret"); err != nil {
		t.Fatalf("SetAdminToken: %v", err)
	}
	apperrtest.WantKind(t, e.svc.Create(ctx, root, "orphan"), apperr.Unauthenticated, "invalid token")
	if exists, err := spaceExists(ctx, e.db, "orphan"); exists || err != nil {
		t.Fatalf("orphan exists = %v, %v; want false", exists, err)
	}
}

// wantAdmins asserts the token count of every admin agent.
func wantAdmins(t *testing.T, e env, root auth.Principal, want map[string]int) {
	t.Helper()
	admins, err := e.auth.ListAdmins(context.Background(), root)
	if err != nil {
		t.Fatalf("ListAdmins: %v", err)
	}
	got := map[string]int{}
	for _, a := range admins {
		got[a.Name] = a.Tokens
	}
	if !maps.Equal(got, want) {
		t.Fatalf("admin tokens = %v, want %v", got, want)
	}
}
