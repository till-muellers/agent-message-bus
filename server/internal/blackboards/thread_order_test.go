package blackboards

import (
	"context"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// steppedClock returns the given instants in order, one per call.
func steppedClock(t *testing.T, at ...time.Time) func() time.Time {
	t.Helper()
	return func() time.Time {
		if len(at) == 0 {
			t.Fatal("clock read more often than expected")
		}
		now := at[0]
		at = at[1:]
		return now
	}
}

// threadsWithTrimmedStamps posts thread 1 at .300, thread 2 at .390 and a reply on
// thread 2 at .3905 within one second: stored as "….3Z", "….39Z" and "….3905Z".
// Their text order ("….3Z" > "….39Z" > "….3905Z") is the reverse of their time order.
func threadsWithTrimmedStamps(t *testing.T) (fixture, int64, int64) {
	t.Helper()
	f := newFixture(t)
	sec := time.Date(2026, 11, 6, 7, 0, 0, 0, time.UTC)
	f.svc = NewService(f.db, spaces.NewResolver(), f.ib, steppedClock(t, sec, // the board
		sec.Add(300*time.Millisecond), sec.Add(390*time.Millisecond), sec.Add(390500*time.Microsecond)))
	ctx := context.Background()
	must(t, f.svc.Create(ctx, f.lead, "alpha", "b"))
	older, err := f.svc.CreateThread(ctx, f.lead, "alpha", "b", "older", "p1")
	must(t, err)
	newer, err := f.svc.CreateThread(ctx, f.lead, "alpha", "b", "newer", "p2")
	must(t, err)
	_, _, err = f.svc.AddPost(ctx, f.lead, "alpha", newer, "p3")
	must(t, err)
	return f, older, newer
}

const newestPostAt = "2026-11-06T07:00:00.3905Z"

func TestListThreadsOrdersByNewestPostNotStampText(t *testing.T) {
	t.Parallel()
	f, older, newer := threadsWithTrimmedStamps(t)
	list, err := f.svc.ListThreads(context.Background(), f.lead, "alpha", "b")
	must(t, err)
	if len(list) != 2 || list[0].ID != newer || list[1].ID != older {
		t.Fatalf("ListThreads = %+v, want thread %d first", list, newer)
	}
	if list[0].LastPostAt != newestPostAt {
		t.Fatalf("lastPostAt = %q, want %q", list[0].LastPostAt, newestPostAt)
	}
}

func TestRecentThreadsOrdersByNewestPostNotStampText(t *testing.T) {
	t.Parallel()
	f, older, newer := threadsWithTrimmedStamps(t)
	m := spaces.Membership{SpaceID: 10, AgentID: 1}
	list, err := f.svc.RecentThreadsFor(context.Background(), m, 10)
	must(t, err)
	if len(list) != 2 || list[0].ID != newer || list[1].ID != older {
		t.Fatalf("RecentThreadsFor = %+v, want thread %d first", list, newer)
	}
	if list[0].LastPostAt != newestPostAt {
		t.Fatalf("lastPostAt = %q, want %q", list[0].LastPostAt, newestPostAt)
	}
}
