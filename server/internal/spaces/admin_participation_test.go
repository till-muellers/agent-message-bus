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

func mustJoin(t *testing.T, e env, p auth.Principal, space string) {
	t.Helper()
	if err := e.svc.Join(context.Background(), p, space); err != nil {
		t.Fatalf("%s joins %s: %v", p.AgentName, space, err)
	}
}

func wantParticipants(t *testing.T, e env, space string, want []Participant) {
	t.Helper()
	got, err := listParticipants(context.Background(), e.db, spaceID(t, e, space))
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("participants of %s = %+v, %v; want %+v", space, got, err, want)
	}
}

func spaceID(t *testing.T, e env, space string) int64 {
	t.Helper()
	id, found, err := spaceIDByName(context.Background(), e.db, space)
	if err != nil || !found {
		t.Fatalf("space %s: found %v, %v", space, found, err)
	}
	return id
}

func TestAdminCreatesLeaderlessSpace(t *testing.T) {
	t.Parallel()
	hook := &creationHook{}
	e := newEnv(t, nil, []CreationHook{hook})
	ops := opsAdmin(t, e)
	mustCreate(t, e, ops, "gamma")
	wantParticipants(t, e, "gamma", []Participant{{"ops", RoleAdmin}})
	if len(hook.calls) != 1 || hook.calls[0].Role != RoleAdmin || !hook.calls[0].Admin {
		t.Fatalf("creation hook calls = %+v", hook.calls)
	}
}

func TestAdminDeletesAnySpace(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t, nil, nil)
	lead, lead2, ops := leader(t, e, 1, "lead"), leader(t, e, 2, "lead2"), opsAdmin(t, e)
	mustCreate(t, e, lead, "demo")
	mustJoin(t, e, lead2, "demo")
	if err := e.svc.Delete(ctx, ops, "demo"); err != nil {
		t.Fatalf("admin deletes space with leaders: %v", err)
	}
	if n := countRows(t, e.db, `SELECT COUNT(*) FROM spaces WHERE name = 'demo'`); n != 0 {
		t.Fatalf("space demo still exists")
	}
}

func TestJoinedAdminNotOtherLeader(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t, nil, nil)
	lead, ops := leader(t, e, 1, "lead"), opsAdmin(t, e)
	mustCreate(t, e, lead, "demo")
	mustJoin(t, e, ops, "demo")
	apperrtest.WantKind(t, e.svc.Leave(ctx, lead, "demo"), apperr.Conflict, "last leader cannot leave; delete the space instead")
	if err := e.svc.Delete(ctx, lead, "demo"); err != nil {
		t.Fatalf("sole leader deletes despite joined admin: %v", err)
	}
}

func TestAdminJoinLeave(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	hook := &recordingHook{recipient: 1}
	e := newEnv(t, []DepartureHook{hook}, nil)
	lead, ops := leader(t, e, 1, "lead"), opsAdmin(t, e)
	mustCreate(t, e, lead, "demo")
	mustJoin(t, e, ops, "demo")
	apperrtest.WantKind(t, e.svc.Join(ctx, ops, "demo"), apperr.Conflict, "already a participant of demo")
	apperrtest.WantKind(t, e.svc.Join(ctx, ops, "ghost"), apperr.NotFound, "space ghost not found")
	wantParticipants(t, e, "demo", []Participant{{"ops", RoleAdmin}, {"lead", RoleLeader}})
	inMember := auth.Principal{AgentID: ops.AgentID, SpaceID: spaceID(t, e, "demo")}
	seedInbox(t, e, inMember)
	if err := e.svc.Leave(ctx, ops, "demo"); err != nil {
		t.Fatalf("admin leaves: %v", err)
	}
	wantParticipants(t, e, "demo", []Participant{{"lead", RoleLeader}})
	wantNoInbox(t, e, inMember)
	if len(hook.calls) != 1 || hook.calls[0].Role != RoleAdmin {
		t.Fatalf("departure hook calls = %+v", hook.calls)
	}
	if _, err := e.svc.Participants(ctx, ops, "demo"); err != nil {
		t.Fatalf("authority kept after leaving: %v", err)
	}
}

func TestParticipantOrder(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	lead := leader(t, e, 1, "lead")
	ops := opsAdmin(t, e)
	dbtest.SeedAgent(t, e.db, 8, "admin", "admin")
	root := auth.Principal{Kind: auth.Admin, AgentID: 8, AgentName: "admin"}
	mustCreate(t, e, lead, "demo")
	inviteMember(t, e, lead, "demo", "worker")
	mustJoin(t, e, ops, "demo")
	mustJoin(t, e, root, "demo")
	wantParticipants(t, e, "demo", []Participant{{"admin", RoleAdmin}, {"ops", RoleAdmin}, {"lead", RoleLeader}, {"worker", RoleMember}})
}

func TestListSpacesAdmins(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t, nil, nil)
	lead, ops := leader(t, e, 1, "lead"), opsAdmin(t, e)
	mustCreate(t, e, lead, "demo")
	mustCreate(t, e, lead, "other")
	inviteMember(t, e, lead, "demo", "worker")
	mustJoin(t, e, ops, "demo")
	got, err := e.svc.List(ctx, ops)
	want := []SpaceInfo{
		{Name: "demo", Leaders: []string{"lead"}, Admins: []string{"ops"}, Members: 1, Joined: true},
		{Name: "other", Leaders: []string{"lead"}, Admins: []string{}, Members: 0, Joined: false},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("List(ops) = %+v, %v; want %+v", got, err, want)
	}
	who, err := e.svc.Whoami(ctx, ops)
	if err != nil || !reflect.DeepEqual(who.Spaces, []SpaceRole{{"demo", RoleAdmin}}) {
		t.Fatalf("Whoami(ops) = %+v, %v", who, err)
	}
}

func TestRevokeAdminTargetConflict(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	lead, ops := leader(t, e, 1, "lead"), opsAdmin(t, e)
	dbtest.SeedAgent(t, e.db, 7, "web", "admin")
	web := auth.Principal{Kind: auth.Admin, AgentID: 7, AgentName: "web"}
	mustCreate(t, e, lead, "alpha")
	mustJoin(t, e, ops, "alpha")
	for _, p := range []auth.Principal{lead, web} {
		_, err := e.svc.Revoke(context.Background(), p, "alpha", "ops")
		apperrtest.WantKind(t, err, apperr.Conflict, "ops is an admin of space alpha; admins cannot be revoked from a space")
	}
}
