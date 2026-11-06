package spaces

import (
	"context"
	"testing"
)

// TestDeletedSpaceIDIsNotReused: a space created after the newest one was deleted gets a
// new id, so a member token or inbox scope of the deleted space never matches it, even
// for the same member name (agent-message-bus-cdt, space-id-reuse variant).
func TestDeletedSpaceIDIsNotReused(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	lead := leader(t, e, 1, "lead")
	mustCreate(t, e, lead, "oldsp")
	old, _ := inviteMember(t, e, lead, "oldsp", "reusex")
	if err := e.svc.Delete(context.Background(), lead, "oldsp"); err != nil {
		t.Fatalf("deleting oldsp: %v", err)
	}
	mustCreate(t, e, lead, "newsp")
	fresh, _ := inviteMember(t, e, lead, "newsp", "reusex")
	if fresh.SpaceID == old.SpaceID {
		t.Fatalf("newsp reuses the id %d of the deleted space oldsp", old.SpaceID)
	}
	seedInbox(t, e, fresh)
	wantNoInbox(t, e, old)
}
