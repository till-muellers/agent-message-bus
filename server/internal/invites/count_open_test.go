package invites

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// TestCountOpen counts the open invites of several spaces in one read: expired and
// redeemed codes do not count, and a space without open invites is absent.
func TestCountOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	for _, name := range []string{"other", "quiet"} {
		if err := e.spaces.Create(ctx, e.lead, name); err != nil {
			t.Fatalf("creating %s: %v", name, err)
		}
	}
	mustCreate(t, e, "old", nil)
	*e.clock = start.Add(5 * time.Minute)
	accepted(t, e, mustCreate(t, e, "joined", nil).Code)
	mustCreate(t, e, "a", nil)
	mustCreate(t, e, "b", nil)
	if _, err := e.svc.Create(ctx, e.lead, "other", "c", nil); err != nil {
		t.Fatalf("inviting into other: %v", err)
	}
	*e.clock = start.Add(10 * time.Minute)
	ids := map[string]int64{}
	for _, name := range []string{"demo", "other", "quiet"} {
		ids[name] = spaceIDOf(t, e, name)
	}
	got, err := e.svc.CountOpen(ctx, []int64{ids["demo"], ids["other"], ids["quiet"]})
	if want := map[int64]int{ids["demo"]: 2, ids["other"]: 1}; err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("CountOpen = %v, %v; want %v", got, err, want)
	}
	if got, err := e.svc.CountOpen(ctx, nil); err != nil || len(got) != 0 {
		t.Fatalf("CountOpen(nil) = %v, %v; want empty", got, err)
	}
}

// TestListOpenMatchesListPending: ListOpen is ListPending for a space id the caller has
// already authorized.
func TestListOpenMatchesListPending(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	mustCreate(t, e, "a", nil)
	mustCreate(t, e, "b", nil)
	got, err := e.svc.ListOpen(context.Background(), spaceIDOf(t, e, "demo"))
	if want := listPending(t, e, e.lead, "demo"); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("ListOpen = %+v, %v; want %+v", got, err, want)
	}
}

// spaceIDOf resolves the id of space for its leader lead.
func spaceIDOf(t *testing.T, e env, space string) int64 {
	t.Helper()
	m, err := spaces.NewResolver(anyCredential{}).Resolve(context.Background(), e.db, e.lead, space)
	if err != nil {
		t.Fatalf("resolving %s: %v", space, err)
	}
	return m.SpaceID
}
