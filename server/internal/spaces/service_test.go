package spaces

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

var admin = auth.Principal{Kind: auth.Admin}

func TestCreate(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	lead := leader(t, e, 1, "lead")
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
		{name: "invalid name", p: lead, space: "", wantKind: apperr.Invalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := e.svc.Create(context.Background(), tc.p, tc.space)
			if tc.wantKind != "" {
				apperrtest.WantKind(t, err, tc.wantKind, tc.wantMsg)
				return
			}
			if err != nil {
				t.Fatalf("create: %v", err)
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
	e := newEnv(t, nil, nil)
	lead := leader(t, e, 1, "lead")
	other := leader(t, e, 2, "other")
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
		{name: "unknown", p: other, space: "nope", wantKind: apperr.NotFound, wantMsg: "space nope not found"},
		{name: "ok", p: other, space: "alpha"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := e.svc.Join(context.Background(), tc.p, tc.space)
			if tc.wantKind != "" {
				apperrtest.WantKind(t, err, tc.wantKind, tc.wantMsg)
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
	e := newEnv(t, nil, nil)
	lead := leader(t, e, 1, "lead")
	other := leader(t, e, 2, "other")
	mustCreate(t, e, other, "beta")
	mustCreate(t, e, lead, "alpha")
	member, _ := inviteMember(t, e, lead, "alpha", "work")
	alpha := SpaceInfo{Name: "alpha", Leaders: []string{"lead"}, Admins: []string{}, Members: 1}
	beta := SpaceInfo{Name: "beta", Leaders: []string{"other"}, Admins: []string{}}
	joined := func(s SpaceInfo) SpaceInfo { s.Joined = true; return s }
	tests := []struct {
		name     string
		p        auth.Principal
		want     []SpaceInfo
		wantKind apperr.Kind
	}{
		{name: "leader sees all", p: lead, want: []SpaceInfo{joined(alpha), beta}},
		{name: "member sees own", p: member, want: []SpaceInfo{joined(alpha)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := e.svc.List(context.Background(), tc.p)
			if tc.wantKind != "" {
				apperrtest.WantKind(t, err, tc.wantKind, "")
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
	e := newEnv(t, nil, nil)
	ctx := context.Background()
	lead := leader(t, e, 1, "lead")
	other := leader(t, e, 2, "other")
	stranger := leader(t, e, 3, "stranger")
	mustCreate(t, e, lead, "alpha")
	member, tok := inviteMember(t, e, lead, "alpha", "work")
	if err := e.svc.Join(ctx, other, "alpha"); err != nil {
		t.Fatalf("join: %v", err)
	}
	apperrtest.WantKind(t, e.svc.Delete(ctx, member, "alpha"), apperr.Forbidden, "")
	apperrtest.WantKind(t, e.svc.Delete(ctx, stranger, "alpha"), apperr.Forbidden, "not a participant of space alpha")
	apperrtest.WantKind(t, e.svc.Delete(ctx, lead, "alpha"), apperr.Conflict,
		"space alpha has other leaders; they must leave first")
	if err := e.svc.Leave(ctx, other, "alpha"); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if err := e.svc.Delete(ctx, lead, "alpha"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	apperrtest.WantKind(t, e.svc.Delete(ctx, lead, "alpha"), apperr.NotFound, "space alpha not found")
	_, err := e.auth.Authenticate(ctx, tok)
	apperrtest.WantKind(t, err, apperr.Unauthenticated, "")
}

func TestAuthorizeInvite(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	ctx := context.Background()
	lead := leader(t, e, 1, "lead")
	leader(t, e, 2, "boss")
	mustCreate(t, e, lead, "alpha")
	member, _ := inviteMember(t, e, lead, "alpha", "work")
	tests := []struct {
		name     string
		p        auth.Principal
		space    string
		invitee  string
		wantKind apperr.Kind
		wantMsg  string
	}{
		{name: "member caller", p: member, space: "alpha", invitee: "x", wantKind: apperr.Forbidden},
		{name: "leader name", p: lead, space: "alpha", invitee: "boss", wantKind: apperr.Conflict,
			wantMsg: "agent boss is a leader agent"},
		{name: "invalid name", p: lead, space: "alpha", invitee: "Bad Name", wantKind: apperr.Invalid},
		{name: "unknown space", p: lead, space: "nope", invitee: "x", wantKind: apperr.NotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := db.InTx(ctx, e.db, func(tx *sql.Tx) error {
				_, err := e.svc.AuthorizeInvite(ctx, tx, tc.p, tc.space, tc.invitee)
				return err
			})
			apperrtest.WantKind(t, err, tc.wantKind, tc.wantMsg)
		})
	}
	t.Run("authorized", func(t *testing.T) {
		err := db.InTx(ctx, e.db, func(tx *sql.Tx) error {
			_, err := e.svc.AuthorizeInvite(ctx, tx, lead, "alpha", "fresh")
			return err
		})
		if err != nil {
			t.Fatalf("authorize: %v", err)
		}
	})
}

func TestAdmitMember(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	ctx := context.Background()
	lead := leader(t, e, 1, "lead")
	mustCreate(t, e, lead, "alpha")
	member, tok1 := inviteMember(t, e, lead, "alpha", "work")
	if n := countRows(t, e.db, `SELECT COUNT(*) FROM participants WHERE agent_id = ? AND role = 'member'`, member.AgentID); n != 1 {
		t.Fatalf("participant rows = %d", n)
	}
	var space, tok2 string
	err := db.InTx(ctx, e.db, func(tx *sql.Tx) error {
		var err error
		space, tok2, err = e.svc.AdmitMember(ctx, tx, member.SpaceID, "work")
		return err
	})
	if err != nil {
		t.Fatalf("re-admit: %v", err)
	}
	if space != "alpha" || tok2 == tok1 {
		t.Fatalf("re-admit = %q, %q (first token %q)", space, tok2, tok1)
	}
	again, err := e.auth.Authenticate(ctx, tok2)
	if err != nil || again != member {
		t.Fatalf("second token = %+v, %v; want %+v", again, err, member)
	}
	if _, err := e.auth.Authenticate(ctx, tok1); err != nil {
		t.Fatalf("first token: %v", err)
	}
	if n := countRows(t, e.db, `SELECT COUNT(*) FROM participants WHERE agent_id = ?`, member.AgentID); n != 1 {
		t.Fatalf("participant rows = %d", n)
	}
}

func TestLeave(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	ctx := context.Background()
	lead := leader(t, e, 1, "lead")
	mustCreate(t, e, lead, "alpha")
	member, tok := inviteMember(t, e, lead, "alpha", "work")
	apperrtest.WantKind(t, e.svc.Leave(ctx, lead, "alpha"), apperr.Conflict, "last leader cannot leave; delete the space instead")
	apperrtest.WantKind(t, e.svc.Leave(ctx, admin, "alpha"), apperr.Forbidden, "")
	seedInbox(t, e, member)
	if err := e.svc.Leave(ctx, member, "alpha"); err != nil {
		t.Fatalf("member leave: %v", err)
	}
	_, err := e.auth.Authenticate(ctx, tok)
	apperrtest.WantKind(t, err, apperr.Unauthenticated, "")
	wantNoInbox(t, e, member)
}

func TestParticipants(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	ctx := context.Background()
	lead := leader(t, e, 1, "zed")
	other := leader(t, e, 2, "amy")
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
	if _, err = e.svc.Participants(ctx, admin, "alpha"); err != nil {
		t.Fatalf("admin lists participants: %v", err)
	}
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
	e := newEnv(t, []DepartureHook{hook}, nil)
	ctx := context.Background()
	lead := leader(t, e, 1, "lead")
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

// seedInbox delivers one direct message to member through the real inbox service.
func seedInbox(t *testing.T, e env, member auth.Principal) {
	t.Helper()
	err := e.inbox.Deliver(context.Background(), func(*sql.Tx) ([]inbox.Entry, error) {
		return []inbox.Entry{{AgentID: member.AgentID, SpaceID: member.SpaceID, Space: "alpha",
			Type: inbox.DirectMessage, From: "lead", Body: "hi"}}, nil
	})
	if err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if got, err := e.inbox.Read(context.Background(), member, 0, 0); err != nil || len(got) != 1 {
		t.Fatalf("seeded inbox = %+v, %v", got, err)
	}
}

// wantNoInbox asserts member's space inbox rows are gone.
func wantNoInbox(t *testing.T, e env, member auth.Principal) {
	t.Helper()
	if n := countRows(t, e.db, `SELECT COUNT(*) FROM inbox WHERE agent_id = ? AND space_id = ?`,
		member.AgentID, member.SpaceID); n != 0 {
		t.Fatalf("inbox rows = %d, want 0", n)
	}
}

// creationHook records creations, checking each runs inside the creating tx.
type creationHook struct {
	calls   []Membership
	visible bool
	err     error
}

func (h *creationHook) OnSpaceCreated(ctx context.Context, tx db.DBTX, m Membership) error {
	h.calls = append(h.calls, m)
	_, found, err := spaceIDByName(ctx, tx, m.SpaceName)
	h.visible = err == nil && found
	return h.err
}

func TestCreationHook(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	tests := []struct {
		name    string
		hookErr error
		want    bool
	}{{"ok", nil, true}, {"error rolls back", boom, false}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hook := &creationHook{err: tc.hookErr}
			e := newEnv(t, nil, []CreationHook{hook})
			lead := leader(t, e, 1, "lead")
			err := e.svc.Create(context.Background(), lead, "alpha")
			if !errors.Is(err, tc.hookErr) || (tc.hookErr == nil && err != nil) {
				t.Fatalf("create err = %v, want %v", err, tc.hookErr)
			}
			if len(hook.calls) != 1 || !hook.visible {
				t.Fatalf("calls = %+v, visible in tx = %v", hook.calls, hook.visible)
			}
			wantM := Membership{SpaceID: hook.calls[0].SpaceID, SpaceName: "alpha", AgentID: 1, AgentName: "lead", Role: RoleLeader}
			if hook.calls[0] != wantM || wantM.SpaceID == 0 {
				t.Fatalf("membership = %+v, want %+v", hook.calls[0], wantM)
			}
			if n := countRows(t, e.db, `SELECT COUNT(*) FROM spaces WHERE name = 'alpha'`); (n == 1) != tc.want {
				t.Fatalf("spaces = %d, want exists %v", n, tc.want)
			}
		})
	}
}
