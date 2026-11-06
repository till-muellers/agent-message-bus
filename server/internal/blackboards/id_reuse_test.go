package blackboards

import (
	"context"
	"strconv"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
)

// TestRemovedThreadIDIsNotReused: a thread created after the newest one was removed gets
// a new id, so the threadId of the removal entry (or of a report naming it) never opens
// another thread.
func TestRemovedThreadIDIsNotReused(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	removed := boardWithThread(t, f, f.m1)
	must(t, f.svc.Subscribe(ctx, f.m2, "alpha", removed))
	must(t, f.svc.RemoveThread(ctx, f.m1, "alpha", removed))
	next, err := f.svc.CreateThread(ctx, f.lead, "alpha", "b", "next", "first")
	must(t, err)
	if next == removed {
		t.Fatalf("the new thread reuses the id %d of the removed thread", removed)
	}
	es := readInbox(t, f.ib, f.m2)
	if len(es) != 1 || es[0].ThreadID != removed {
		t.Fatalf("m2 inbox = %+v, want the removal of thread %d", es, removed)
	}
	_, err = f.svc.ReadThread(ctx, f.m2, "alpha", es[0].ThreadID)
	apperrtest.WantKind(t, err, apperr.NotFound, "thread "+strconv.FormatInt(removed, 10)+" not found")
}
