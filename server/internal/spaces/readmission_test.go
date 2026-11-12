package spaces

import (
	"context"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// readmissionHook records the memberships it is called for.
type readmissionHook struct{ calls []Membership }

func (h *readmissionHook) OnReadmission(_ context.Context, _ db.DBTX, m Membership) error {
	h.calls = append(h.calls, m)
	return nil
}

// TestReinviteStartsClean: inviting a participating member again drops its tokens and inbox
// entries in that space and runs the readmission hooks; its other spaces are untouched.
func TestReinviteStartsClean(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	hook := &readmissionHook{}
	e.adm = NewAdmitter(NewResolver(e.auth), e.auth, e.inbox, time.Now, []ReadmissionHook{hook})
	lead := leader(t, e, 1, "lead")
	mustCreate(t, e, lead, "alpha")
	mustCreate(t, e, lead, "beta")
	first, firstTok := inviteMember(t, e, lead, "alpha", "worker")
	beta, betaTok := inviteMember(t, e, lead, "beta", "worker")
	seedInbox(t, e, first)
	seedInbox(t, e, beta)
	if len(hook.calls) != 0 {
		t.Fatalf("hook ran on first admissions: %+v", hook.calls)
	}
	again, _ := inviteMember(t, e, lead, "alpha", "worker")
	_, err := e.auth.Authenticate(context.Background(), firstTok)
	apperrtest.WantKind(t, err, apperr.Unauthenticated, "invalid token")
	if _, err := e.auth.Authenticate(context.Background(), betaTok); err != nil {
		t.Fatalf("beta token after re-invite into alpha: %v", err)
	}
	if got, err := e.inbox.Read(context.Background(), again, 0, inbox.MaxLimit); err != nil || len(got) != 0 {
		t.Fatalf("alpha inbox after re-invite = %+v, %v; want empty", got, err)
	}
	if got, err := e.inbox.Read(context.Background(), beta, 0, inbox.MaxLimit); err != nil || len(got) != 1 {
		t.Fatalf("beta inbox after re-invite into alpha = %+v, %v; want its entry", got, err)
	}
	want := Membership{SpaceID: first.SpaceID, SpaceName: "alpha", AgentID: first.AgentID, AgentName: "worker", Role: RoleMember}
	if len(hook.calls) != 1 || hook.calls[0] != want {
		t.Fatalf("readmission hook calls = %+v, want [%+v]", hook.calls, want)
	}
	wantParticipants(t, e, "alpha", []Participant{{Name: "lead", Role: RoleLeader}, {Name: "worker", Role: RoleMember}})
}
