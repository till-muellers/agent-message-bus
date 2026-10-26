package spaces

import (
	"context"
	"reflect"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

// opsAdmin seeds the admin agent "ops" with id 9 and returns its principal.
func opsAdmin(t *testing.T, e env) auth.Principal {
	t.Helper()
	dbtest.SeedAgent(t, e.db, 9, "ops", "admin")
	return auth.Principal{Kind: auth.Admin, AgentID: 9, AgentName: "ops"}
}

func TestResolveAdmin(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	lead := leader(t, e, 1, "lead")
	mustCreate(t, e, lead, "alpha")
	ops := opsAdmin(t, e)
	got, err := e.svc.r.Resolve(context.Background(), ops, "alpha")
	want := Membership{SpaceID: got.SpaceID, SpaceName: "alpha", AgentID: 9, AgentName: "ops", Admin: true}
	if err != nil || got != want || got.SpaceID == 0 || got.Joined() || !got.Leads() {
		t.Fatalf("Resolve(ops) = %+v, %v; want %+v", got, err, want)
	}
	_, err = e.svc.r.Resolve(context.Background(), ops, "ghost")
	apperrtest.WantKind(t, err, apperr.NotFound, "space ghost not found")
}

func TestAdminRevokesLeaders(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t, nil, nil)
	lead := leader(t, e, 1, "lead")
	lead2 := leader(t, e, 2, "lead2")
	ops := opsAdmin(t, e)
	mustCreate(t, e, lead, "alpha")
	if err := e.svc.Join(ctx, lead2, "alpha"); err != nil {
		t.Fatalf("join: %v", err)
	}
	inviteMember(t, e, lead, "alpha", "worker")
	_, err := e.svc.Revoke(ctx, lead, "alpha", "lead2")
	apperrtest.WantKind(t, err, apperr.Conflict, "lead2 is a leader of space alpha; leaders cannot be revoked")
	wantRevoke(t, e, ops, "lead2", true)
	wantRevoke(t, e, ops, "lead", true)
	ps, err := e.svc.Participants(ctx, ops, "alpha")
	if want := []Participant{{"worker", RoleMember, false}}; err != nil || !reflect.DeepEqual(ps, want) {
		t.Fatalf("participants after revoking the last leader = %+v, %v; want %+v", ps, err, want)
	}
	if err := e.svc.Join(ctx, lead, "alpha"); err != nil {
		t.Fatalf("revoked leader re-joins: %v", err)
	}
}

func TestAdminInvitesAndLeaves(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t, nil, nil)
	lead := leader(t, e, 1, "lead")
	ops := opsAdmin(t, e)
	mustCreate(t, e, lead, "alpha")
	inviteMember(t, e, ops, "alpha", "helper")
	apperrtest.WantKind(t, e.svc.Leave(ctx, ops, "alpha"), apperr.Forbidden, "not a participant of space alpha")
}
