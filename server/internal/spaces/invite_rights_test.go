package spaces

import (
	"context"
	"reflect"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
)

// TestMaySeeInvites is the one rule for who may see a space's pending invites (leader
// rights: admins in every space, leaders where they lead, never members), asked of each
// space Listing returns and of the resolved membership.
func TestMaySeeInvites(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	lead, lead2, ops := leader(t, e, 1, "lead"), leader(t, e, 2, "lead2"), opsAdmin(t, e)
	mustCreate(t, e, lead, "demo")
	mustCreate(t, e, lead2, "other")
	mustJoin(t, e, ops, "other")
	worker, _ := inviteMember(t, e, lead, "demo", "worker")
	tests := []struct {
		name  string
		p     auth.Principal
		space string
		want  bool
	}{
		{"unjoined admin", ops, "demo", true},
		{"joined admin", ops, "other", true},
		{"bootstrap admin", instanceAdmin(t, e), "demo", true},
		{"leader of the space", lead, "demo", true},
		{"leader of another space", lead, "other", false},
		{"member", worker, "demo", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := listedMaySee(t, e, tc.p, tc.space); got != tc.want {
				t.Fatalf("Listing: MaySeeInvites = %v, want %v", got, tc.want)
			}
			m, err := e.svc.r.Resolve(context.Background(), e.db, tc.p, tc.space)
			if err == nil && m.MaySeeInvites() != tc.want {
				t.Fatalf("Resolve: MaySeeInvites = %v, want %v", m.MaySeeInvites(), tc.want)
			}
		})
	}
}

// listedMaySee returns MaySeeInvites of space as Listing reports it to p, after checking
// that Listing carries the same infos as List.
func listedMaySee(t *testing.T, e env, p auth.Principal, space string) bool {
	t.Helper()
	listed, err := e.svc.Listing(context.Background(), p)
	if err != nil {
		t.Fatalf("Listing: %v", err)
	}
	infos, err := e.svc.List(context.Background(), p)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := make([]SpaceInfo, 0, len(listed))
	for _, l := range listed {
		got = append(got, l.SpaceInfo)
	}
	if !reflect.DeepEqual(got, infos) {
		t.Fatalf("Listing infos = %+v, List = %+v", got, infos)
	}
	for _, l := range listed {
		if l.Name == space {
			return l.Caller.MaySeeInvites()
		}
	}
	t.Fatalf("space %s not listed for %s", space, p.AgentName)
	return false
}
