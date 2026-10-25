package spaces

import (
	"context"
	"reflect"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

var admin = auth.Principal{Kind: auth.Admin}

func TestCreate(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	lead := seedLeader(t, e.db, "lead")
	mustCreate(t, e, lead, "alpha")
	member, _ := inviteMember(t, e, lead, "alpha", "work")
	tests := []struct {
		name     string
		p        auth.Principal
		space    string
		wantKind apperr.Kind
		wantMsg  string
	}{
		{name: "ok", p: lead, space: "beta"},
		{name: "duplicate", p: lead, space: "alpha", wantKind: apperr.Conflict, wantMsg: "space alpha already exists"},
		{name: "member", p: member, space: "gamma", wantKind: apperr.Forbidden},
		{name: "admin", p: admin, space: "gamma", wantKind: apperr.Forbidden},
		{name: "invalid name", p: lead, space: "", wantKind: apperr.Invalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := e.svc.Create(context.Background(), tc.p, tc.space)
			if tc.wantKind != "" {
				assertErr(t, err, tc.wantKind, tc.wantMsg)
				return
			}
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			if n := countRows(t, e.db, `SELECT COUNT(*) FROM topics t JOIN spaces s ON s.id = t.space_id
				WHERE s.name = ? AND t.name = 'all' AND t.closed_at IS NULL AND t.created_by = ?`, tc.space, tc.p.AgentID); n != 1 {
				t.Fatalf("all topics = %d, want 1", n)
			}
			ps, _ := e.svc.Participants(context.Background(), tc.p, tc.space)
			if want := []Participant{{"lead", RoleLeader}}; !reflect.DeepEqual(ps, want) {
				t.Fatalf("participants = %+v", ps)
			}
		})
	}
}

func TestJoin(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	lead := seedLeader(t, e.db, "lead")
	other := seedLeader(t, e.db, "other")
	mustCreate(t, e, lead, "alpha")
	member, _ := inviteMember(t, e, lead, "alpha", "work")
	tests := []struct {
		name     string
		p        auth.Principal
		space    string
		wantKind apperr.Kind
		wantMsg  string
	}{
		{name: "already participant", p: lead, space: "alpha", wantKind: apperr.Conflict,
			wantMsg: "already a participant of alpha"},
		{name: "member", p: member, space: "alpha", wantKind: apperr.Forbidden},
		{name: "admin", p: admin, space: "alpha", wantKind: apperr.Forbidden},
		{name: "unknown", p: other, space: "nope", wantKind: apperr.NotFound, wantMsg: "space nope not found"},
		{name: "ok", p: other, space: "alpha"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := e.svc.Join(context.Background(), tc.p, tc.space)
			if tc.wantKind != "" {
				assertErr(t, err, tc.wantKind, tc.wantMsg)
				return
			}
			if err != nil {
				t.Fatalf("join: %v", err)
			}
			ps, _ := e.svc.Participants(context.Background(), tc.p, tc.space)
			want := []Participant{{"lead", RoleLeader}, {"other", RoleLeader}, {"work", RoleMember}}
			if !reflect.DeepEqual(ps, want) {
				t.Fatalf("participants = %+v, want %+v", ps, want)
			}
		})
	}
}

func TestList(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	lead := seedLeader(t, e.db, "lead")
	other := seedLeader(t, e.db, "other")
	mustCreate(t, e, other, "beta")
	mustCreate(t, e, lead, "alpha")
	member, _ := inviteMember(t, e, lead, "alpha", "work")
	alpha := SpaceInfo{Name: "alpha", Leaders: []string{"lead"}, Members: 1}
	beta := SpaceInfo{Name: "beta", Leaders: []string{"other"}}
	joined := func(s SpaceInfo) SpaceInfo { s.Joined = true; return s }
	tests := []struct {
		name     string
		p        auth.Principal
		want     []SpaceInfo
		wantKind apperr.Kind
	}{
		{name: "leader sees all", p: lead, want: []SpaceInfo{joined(alpha), beta}},
		{name: "member sees own", p: member, want: []SpaceInfo{joined(alpha)}},
		{name: "admin", p: admin, wantKind: apperr.Forbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := e.svc.List(context.Background(), tc.p)
			if tc.wantKind != "" {
				assertErr(t, err, tc.wantKind, "")
				return
			}
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	lead := seedLeader(t, e.db, "lead")
	other := seedLeader(t, e.db, "other")
	stranger := seedLeader(t, e.db, "stranger")
	mustCreate(t, e, lead, "alpha")
	member, tok := inviteMember(t, e, lead, "alpha", "work")
	if err := e.svc.Join(ctx, other, "alpha"); err != nil {
		t.Fatalf("join: %v", err)
	}
	assertErr(t, e.svc.Delete(ctx, member, "alpha"), apperr.Forbidden, "")
	assertErr(t, e.svc.Delete(ctx, stranger, "alpha"), apperr.Forbidden, "not a participant of space alpha")
	assertErr(t, e.svc.Delete(ctx, lead, "alpha"), apperr.Conflict,
		"space alpha has other leaders; they must leave first")
	if err := e.svc.Leave(ctx, other, "alpha"); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if err := e.svc.Delete(ctx, lead, "alpha"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	assertErr(t, e.svc.Delete(ctx, lead, "alpha"), apperr.NotFound, "space alpha not found")
	_, err := e.auth.Authenticate(ctx, tok)
	assertErr(t, err, apperr.Unauthenticated, "")
}

func TestInvite(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	lead := seedLeader(t, e.db, "lead")
	seedLeader(t, e.db, "boss")
	mustCreate(t, e, lead, "alpha")
	member, tok1 := inviteMember(t, e, lead, "alpha", "work")
	tests := []struct {
		name     string
		p        auth.Principal
		space    string
		invitee  string
		wantKind apperr.Kind
		wantMsg  string
	}{
		{name: "member caller", p: member, space: "alpha", invitee: "x", wantKind: apperr.Forbidden},
		{name: "admin", p: admin, space: "alpha", invitee: "x", wantKind: apperr.Forbidden},
		{name: "leader name", p: lead, space: "alpha", invitee: "boss", wantKind: apperr.Conflict,
			wantMsg: "agent boss is a leader agent"},
		{name: "unknown space", p: lead, space: "nope", invitee: "x", wantKind: apperr.NotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.Invite(ctx, tc.p, tc.space, tc.invitee)
			assertErr(t, err, tc.wantKind, tc.wantMsg)
		})
	}
	t.Run("re-invite", func(t *testing.T) {
		again, tok2 := inviteMember(t, e, lead, "alpha", "work")
		if tok2 == tok1 || again != member {
			t.Fatalf("second token %q (%+v), first %q (%+v)", tok2, again, tok1, member)
		}
		if _, err := e.auth.Authenticate(ctx, tok1); err != nil {
			t.Fatalf("first token: %v", err)
		}
		if n := countRows(t, e.db, `SELECT COUNT(*) FROM participants WHERE agent_id = ?`, member.AgentID); n != 1 {
			t.Fatalf("participant rows = %d", n)
		}
	})
}

func TestRevoke(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	lead := seedLeader(t, e.db, "lead")
	other := seedLeader(t, e.db, "other")
	mustCreate(t, e, lead, "alpha")
	if err := e.svc.Join(ctx, other, "alpha"); err != nil {
		t.Fatalf("join: %v", err)
	}
	member, tok := inviteMember(t, e, lead, "alpha", "work")
	peer, _ := inviteMember(t, e, lead, "alpha", "peer")
	mustExec(t, e.db, `INSERT INTO inbox (agent_id, space_id, type, from_name, created_at) VALUES (?, ?, 'direct_message', 'lead', ?)`,
		member.AgentID, member.SpaceID, fixtureTS)
	assertErr(t, e.svc.Revoke(ctx, peer, "alpha", "work"), apperr.Forbidden, "")
	assertErr(t, e.svc.Revoke(ctx, lead, "alpha", "other"), apperr.NotFound, "member other not in space alpha")
	assertErr(t, e.svc.Revoke(ctx, lead, "alpha", "ghost"), apperr.NotFound, "member ghost not in space alpha")
	if err := e.svc.Revoke(ctx, lead, "alpha", "work"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	_, err := e.auth.Authenticate(ctx, tok)
	assertErr(t, err, apperr.Unauthenticated, "")
	if n := countRows(t, e.db, `SELECT COUNT(*) FROM inbox WHERE agent_id = ?`, member.AgentID); n != 0 {
		t.Fatalf("inbox rows = %d", n)
	}
	ps, _ := e.svc.Participants(ctx, lead, "alpha")
	want := []Participant{{"lead", RoleLeader}, {"other", RoleLeader}, {"peer", RoleMember}}
	if !reflect.DeepEqual(ps, want) {
		t.Fatalf("participants = %+v", ps)
	}
}

func TestLeave(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	lead := seedLeader(t, e.db, "lead")
	mustCreate(t, e, lead, "alpha")
	member, tok := inviteMember(t, e, lead, "alpha", "work")
	assertErr(t, e.svc.Leave(ctx, lead, "alpha"), apperr.Conflict, "last leader cannot leave; delete the space instead")
	assertErr(t, e.svc.Leave(ctx, admin, "alpha"), apperr.Forbidden, "")
	if err := e.svc.Leave(ctx, member, "alpha"); err != nil {
		t.Fatalf("member leave: %v", err)
	}
	_, err := e.auth.Authenticate(ctx, tok)
	assertErr(t, err, apperr.Unauthenticated, "")
}

func TestParticipants(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	lead := seedLeader(t, e.db, "zed")
	other := seedLeader(t, e.db, "amy")
	mustCreate(t, e, lead, "alpha")
	if err := e.svc.Join(ctx, other, "alpha"); err != nil {
		t.Fatalf("join: %v", err)
	}
	member, _ := inviteMember(t, e, lead, "alpha", "bob")
	inviteMember(t, e, lead, "alpha", "abe")
	got, err := e.svc.Participants(ctx, member, "alpha")
	want := []Participant{{"amy", RoleLeader}, {"zed", RoleLeader}, {"abe", RoleMember}, {"bob", RoleMember}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, %v; want %+v", got, err, want)
	}
	_, err = e.svc.Participants(ctx, admin, "alpha")
	assertErr(t, err, apperr.Forbidden, "")
}

// recordingHook records departures and notifies a fixed recipient.
type recordingHook struct {
	recipient int64
	calls     []Membership
}

func (h *recordingHook) OnDeparture(_ context.Context, _ db.DBTX, m Membership) ([]inbox.Entry, error) {
	h.calls = append(h.calls, m)
	return []inbox.Entry{{AgentID: h.recipient, SpaceID: m.SpaceID, Space: m.SpaceName,
		Type: inbox.BlackboardRemoved, From: m.AgentName, Blackboard: "notes"}}, nil
}

func TestDepartureHook(t *testing.T) {
	t.Parallel()
	hook := &recordingHook{}
	e := newEnv(t, hook)
	ctx := context.Background()
	lead := seedLeader(t, e.db, "lead")
	mustCreate(t, e, lead, "alpha")
	member, _ := inviteMember(t, e, lead, "alpha", "work")
	hook.recipient = lead.AgentID
	if err := e.svc.Leave(ctx, member, "alpha"); err != nil {
		t.Fatalf("leave: %v", err)
	}
	wantM := Membership{SpaceID: member.SpaceID, SpaceName: "alpha", AgentID: member.AgentID, AgentName: "work", Role: RoleMember}
	if len(hook.calls) != 1 || hook.calls[0] != wantM {
		t.Fatalf("calls = %+v, want %+v", hook.calls, wantM)
	}
	got, err := e.inbox.Read(ctx, lead, 0, 0)
	if err != nil || len(got) != 1 || got[0].Type != inbox.BlackboardRemoved || got[0].From != "work" ||
		got[0].Space != "alpha" || got[0].Blackboard != "notes" {
		t.Fatalf("inbox = %+v, %v", got, err)
	}
}
