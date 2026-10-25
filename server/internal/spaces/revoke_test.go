package spaces

import (
	"context"
	"reflect"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// pendingHook holds pending names per space id and drops them on revocation.
type pendingHook struct{ pending map[int64][]string }

func (h *pendingHook) OnRevoke(_ context.Context, _ db.DBTX, spaceID int64, name string) (bool, error) {
	for i, n := range h.pending[spaceID] {
		if n == name {
			h.pending[spaceID] = append(h.pending[spaceID][:i], h.pending[spaceID][i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func TestRevoke(t *testing.T) {
	t.Parallel()
	hook := &pendingHook{pending: map[int64][]string{}}
	e := newEnvHooks(t, Hooks{Revocation: []RevocationHook{hook}})
	ctx := context.Background()
	lead := leader(t, e, 1, "lead")
	mustCreate(t, e, lead, "alpha")
	member, tok := inviteMember(t, e, lead, "alpha", "work")
	hook.pending[member.SpaceID] = []string{"work", "pend"}
	seedInbox(t, e, member)
	wantRevoke(t, e, lead, "work", true)
	_, err := e.auth.Authenticate(ctx, tok)
	apperrtest.WantKind(t, err, apperr.Unauthenticated, "")
	wantNoInbox(t, e, member)
	if len(hook.pending[member.SpaceID]) != 1 {
		t.Fatalf("pending after revoking work = %v", hook.pending)
	}
	wantRevoke(t, e, lead, "pend", true)
	wantRevoke(t, e, lead, "ghost", false)
	wantRevoke(t, e, lead, "work", false)
	wantRevoke(t, e, lead, "pend", false)
}

func TestRevokeRefusals(t *testing.T) {
	t.Parallel()
	hook := &countingHook{}
	e := newEnvHooks(t, Hooks{Revocation: []RevocationHook{hook}})
	ctx := context.Background()
	lead := leader(t, e, 1, "lead")
	other := leader(t, e, 2, "other")
	mustCreate(t, e, lead, "alpha")
	if err := e.svc.Join(ctx, other, "alpha"); err != nil {
		t.Fatalf("join: %v", err)
	}
	peer, _ := inviteMember(t, e, lead, "alpha", "peer")
	_, err := e.svc.Revoke(ctx, peer, "alpha", "ghost")
	apperrtest.WantKind(t, err, apperr.Forbidden, "only leaders may revoke members")
	_, err = e.svc.Revoke(ctx, lead, "nope", "peer")
	apperrtest.WantKind(t, err, apperr.NotFound, "space nope not found")
	deleted, err := e.svc.Revoke(ctx, lead, "alpha", "other")
	apperrtest.WantKind(t, err, apperr.Conflict, "other is a leader of space alpha; leaders cannot be revoked")
	if deleted || hook.calls != 0 {
		t.Fatalf("revoke leader: deleted = %v, hook calls = %d; want false, 0", deleted, hook.calls)
	}
	ps, _ := e.svc.Participants(ctx, lead, "alpha")
	want := []Participant{{"lead", RoleLeader}, {"other", RoleLeader}, {"peer", RoleMember}}
	if !reflect.DeepEqual(ps, want) {
		t.Fatalf("participants = %+v", ps)
	}
}

func wantRevoke(t *testing.T, e env, p auth.Principal, name string, want bool) {
	t.Helper()
	got, err := e.svc.Revoke(context.Background(), p, "alpha", name)
	if err != nil || got != want {
		t.Fatalf("revoke %s = %v, %v; want %v", name, got, err, want)
	}
}

// countingHook reports a removal on every call and counts its calls.
type countingHook struct{ calls int }

func (h *countingHook) OnRevoke(context.Context, db.DBTX, int64, string) (bool, error) {
	h.calls++
	return true, nil
}
