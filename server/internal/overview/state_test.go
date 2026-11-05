package overview_test

import (
	"reflect"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
)

// guiSetup: lead2 creates space gamma; lead invites "pending" into demo; worker sends lead
// one direct message.
func guiSetup(t *testing.T, w *world) {
	t.Helper()
	w.must(t, w.sp.Create(t.Context(), w.lead2, "gamma"))
	_, err := w.inv.Create(t.Context(), w.lead, "demo", "pending", nil)
	w.must(t, err)
	_, err = w.msg.SendDirect(t.Context(), w.wkr, "demo", "lead", "hi", false)
	w.must(t, err)
}

// TestListState: the GUI space list counts open invites only where the caller may see
// them, and carries the caller and its waiting entries.
func TestListState(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	guiSetup(t, w)
	n := func(v int) *int { return &v }
	tests := []struct {
		name    string
		p       auth.Principal
		invites map[string]*int
		waiting int
	}{
		{"leader: counts where it leads", w.lead, map[string]*int{"beta": n(0), "demo": n(1), "gamma": nil}, 1},
		{"admin: counts everywhere", w.ops, map[string]*int{"beta": n(0), "demo": n(1), "gamma": n(0)}, 0},
		{"member: its space, no count", w.wkr, map[string]*int{"demo": nil}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, err := w.ov.ListState(t.Context(), tc.p)
			w.must(t, err)
			got := map[string]*int{}
			for _, row := range st.Spaces {
				got[row.Name] = row.Invites
			}
			if !reflect.DeepEqual(got, tc.invites) || st.Waiting != tc.waiting || st.You.Name != tc.p.AgentName {
				t.Fatalf("ListState = %+v (invites %v), want invites %v, waiting %d", st, got, tc.invites, tc.waiting)
			}
		})
	}
}

// TestSpaceState: the GUI space view carries the overview, the open invites only for
// callers that may see them, and the waiting entries; the overview gate still applies.
func TestSpaceState(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	guiSetup(t, w)
	tests := []struct {
		name    string
		p       auth.Principal
		invites []string // nil: no invites
		waiting int
	}{
		{"leader of the space", w.lead, []string{"pending"}, 1},
		{"unjoined admin", w.ops, []string{"pending"}, 0},
		{"member", w.wkr, nil, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, err := w.ov.SpaceState(t.Context(), tc.p, "demo")
			w.must(t, err)
			var got []string
			if st.Invites != nil {
				got = []string{}
				for _, inv := range *st.Invites {
					got = append(got, inv.Name)
				}
			}
			if !reflect.DeepEqual(got, tc.invites) || st.Waiting != tc.waiting || st.Overview.Space != "demo" ||
				st.You.Name != tc.p.AgentName {
				t.Fatalf("SpaceState = %+v (invites %v), want invites %v, waiting %d", st, got, tc.invites, tc.waiting)
			}
		})
	}
	_, err := w.ov.SpaceState(t.Context(), w.lead2, "demo")
	apperrtest.WantKind(t, err, apperr.Forbidden, "not a participant of space demo")
}

// TestOverviewResolvesOnce: one overview resolves the caller's membership once and reads
// the feature services scoped to it.
func TestOverviewResolvesOnce(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	memberViewSetup(t, w)
	w.resolves.Store(0)
	_, err := w.ov.Space(t.Context(), w.wkr, "demo")
	w.must(t, err)
	if n := w.resolves.Load(); n != 1 {
		t.Fatalf("Resolve calls = %d, want 1", n)
	}
}
