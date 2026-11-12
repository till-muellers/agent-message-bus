package spaces

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

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
			if want := []Participant{{Name: "lead", Role: RoleLeader}}; !reflect.DeepEqual(roster(ps), want) {
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
			ps = roster(ps)
			want := []Participant{{Name: "lead", Role: RoleLeader}, {Name: "other", Role: RoleLeader}, {Name: "work", Role: RoleMember}}
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
				_, err := e.adm.AuthorizeInvite(ctx, tx, tc.p, tc.space, tc.invitee)
				return err
			})
			apperrtest.WantKind(t, err, tc.wantKind, tc.wantMsg)
		})
	}
	t.Run("authorized", func(t *testing.T) {
		err := db.InTx(ctx, e.db, func(tx *sql.Tx) error {
			_, err := e.adm.AuthorizeInvite(ctx, tx, lead, "alpha", "fresh")
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
	alpha := []Participant{{Name: "lead", Role: RoleLeader}, {Name: "work", Role: RoleMember}}
	wantParticipants(t, e, "alpha", alpha)
	var agentID int64
	var space, tok2 string
	err := db.InTx(ctx, e.db, func(tx *sql.Tx) error {
		var err error
		agentID, space, tok2, err = e.adm.AdmitMember(ctx, tx, member.SpaceID, "work")
		return err
	})
	if err != nil {
		t.Fatalf("re-admit: %v", err)
	}
	if agentID != member.AgentID || space != "alpha" || tok2 == tok1 {
		t.Fatalf("re-admit = %d, %q, %q (first token %q)", agentID, space, tok2, tok1)
	}
	again, err := e.auth.Authenticate(ctx, tok2)
	if err != nil || again.Kind != auth.Member || again.AgentID != member.AgentID || again.SpaceID != member.SpaceID {
		t.Fatalf("second token = %+v, %v; want the identity of %+v", again, err, member)
	}
	if _, err := e.auth.Authenticate(ctx, tok1); err != nil {
		t.Fatalf("first token: %v", err)
	}
	wantParticipants(t, e, "alpha", alpha)
}

func TestLeave(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	ctx := context.Background()
	lead := leader(t, e, 1, "lead")
	mustCreate(t, e, lead, "alpha")
	member, tok := inviteMember(t, e, lead, "alpha", "work")
	apperrtest.WantKind(t, e.svc.Leave(ctx, lead, "alpha"), apperr.Conflict, "last leader cannot leave; delete the space instead")
	apperrtest.WantKind(t, e.svc.Leave(ctx, instanceAdmin(t, e), "alpha"), apperr.Forbidden, "")
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
	want := []Participant{{Name: "amy", Role: RoleLeader}, {Name: "zed", Role: RoleLeader}, {Name: "abe", Role: RoleMember}, {Name: "bob", Role: RoleMember}}
	if err != nil || !reflect.DeepEqual(roster(got), want) {
		t.Fatalf("got %+v, %v; want %+v", got, err, want)
	}
	if _, err = e.svc.Participants(ctx, instanceAdmin(t, e), "alpha"); err != nil {
		t.Fatalf("admin lists participants: %v", err)
	}
}

// presenceStub reports the agents in connected as connected, every other one as offline
// since stubOfflineSince.
type presenceStub struct {
	*inbox.Service
	connected map[int64]bool
}

var stubOfflineSince = time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)

func (p presenceStub) OfflineSince(agentID int64) (time.Time, bool) {
	if p.connected[agentID] {
		return time.Time{}, false
	}
	return stubOfflineSince, true
}

// roster drops each participant's offlineSince, which is the inbox's start time here.
func roster(ps []Participant) []Participant {
	out := make([]Participant, len(ps))
	for i, p := range ps {
		p.OfflineSince = ""
		out[i] = p
	}
	return out
}

// presenceOf maps each participant to "connected" or its offlineSince; a connected one
// that also carries offlineSince shows both.
func presenceOf(ps []Participant) map[string]string {
	out := map[string]string{}
	for _, p := range ps {
		out[p.Name] = p.OfflineSince
		if p.Connected {
			out[p.Name] = "connected" + p.OfflineSince
		}
	}
	return out
}

func TestParticipantsPresence(t *testing.T) {
	t.Parallel()
	offline := "2026-11-12T09:00:00Z"
	tests := []struct {
		name      string
		connected []string
		want      map[string]string // name → "connected" or offlineSince
	}{
		{name: "nobody", want: map[string]string{"lead": offline, "peer": offline, "worker": offline}},
		{name: "peer", connected: []string{"peer"}, want: map[string]string{"lead": offline, "peer": "connected", "worker": offline}},
		{name: "lead of two spaces", connected: []string{"lead"}, want: map[string]string{"lead": "connected", "peer": offline, "worker": offline}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t, nil, nil)
			stub := presenceStub{Service: e.inbox, connected: map[int64]bool{}}
			e.svc = NewService(e.db, NewResolver(e.auth), e.auth, stub, time.Now, Hooks{})
			lead := leader(t, e, 1, "lead")
			mustCreate(t, e, lead, "alpha")
			mustCreate(t, e, lead, "beta")
			worker, _ := inviteMember(t, e, lead, "alpha", "worker")
			peer, _ := inviteMember(t, e, lead, "alpha", "peer")
			ids := map[string]int64{"lead": lead.AgentID, "worker": worker.AgentID, "peer": peer.AgentID}
			for _, n := range tc.connected {
				stub.connected[ids[n]] = true
			}
			ps, err := e.svc.Participants(context.Background(), worker, "alpha")
			if err != nil {
				t.Fatalf("Participants: %v", err)
			}
			if got := presenceOf(ps); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("presence = %v, want %v", got, tc.want)
			}
		})
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
	got, err := e.inbox.Read(ctx, lead, 0, inbox.MaxLimit)
	if err != nil || len(got) != 1 || got[0].Type != inbox.BlackboardRemoved || got[0].From != "work" ||
		got[0].Space != "alpha" || got[0].Blackboard != "notes" {
		t.Fatalf("inbox = %+v, %v", got, err)
	}
}

// seedInbox delivers one direct message to member through the real inbox service.
func seedInbox(t *testing.T, e env, member auth.Principal) {
	t.Helper()
	_, err := inbox.InTx(context.Background(), e.db, e.inbox, func(db.DBTX) ([]inbox.Entry, error) {
		return []inbox.Entry{{AgentID: member.AgentID, SpaceID: member.SpaceID, Space: "alpha",
			Type: inbox.DirectMessage, From: "lead", Body: "hi"}}, nil
	})
	if err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if got, err := e.inbox.Read(context.Background(), member, 0, inbox.MaxLimit); err != nil || len(got) != 1 {
		t.Fatalf("seeded inbox = %+v, %v", got, err)
	}
}

// wantNoInbox asserts that no inbox entry of member's agent remains in member's space;
// a member-scoped principal makes inbox.Count look at that one space.
func wantNoInbox(t *testing.T, e env, member auth.Principal) {
	t.Helper()
	scope := auth.Principal{Kind: auth.Member, AgentID: member.AgentID, SpaceID: member.SpaceID}
	if n, err := e.inbox.Count(context.Background(), scope); err != nil || n != 0 {
		t.Fatalf("inbox entries = %d, %v; want 0", n, err)
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
			wantSpace(t, e, "alpha", tc.want)
		})
	}
}
