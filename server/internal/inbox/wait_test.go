package inbox

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/broker"
)

const longWait = time.Minute

// waitResult is the outcome of one asynchronous Wait.
type waitResult struct {
	entries []Entry
	err     error
}

// startWait runs Wait in the background and returns the channel its result arrives on.
func startWait(ctx context.Context, svc *Service, p auth.Principal, q WaitQuery) <-chan waitResult {
	out := make(chan waitResult, 1)
	go func() {
		es, err := svc.Wait(ctx, p, q)
		out <- waitResult{es, err}
	}()
	return out
}

// result waits for the outcome of a Wait started with startWait.
func result(t *testing.T, ch <-chan waitResult) []string {
	t.Helper()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("Wait: %v", r.err)
		}
		return bodies(r.entries)
	case <-time.After(waitTimeout):
		t.Fatal("Wait did not return")
		return nil
	}
}

// stillWaiting fails when the Wait has already returned.
func stillWaiting(t *testing.T, ch <-chan waitResult) {
	t.Helper()
	select {
	case r := <-ch:
		t.Fatalf("Wait returned %v, %v; want it still waiting", bodies(r.entries), r.err)
	case <-time.After(50 * time.Millisecond):
	}
}

// countingPub counts the live subscriptions currently held through it.
type countingPub struct {
	publisher
	n atomic.Int32
}

func (c *countingPub) Subscribe(topic string) (<-chan []byte, func()) {
	ch, cancel := c.publisher.Subscribe(topic)
	c.n.Add(1)
	var once atomic.Bool
	return ch, func() {
		if once.CompareAndSwap(false, true) {
			c.n.Add(-1)
		}
		cancel()
	}
}

// waitService returns a Service over w's database whose subscriptions through pub are
// counted; root ends its waits. Entries w.send publishes reach it when pub is w.pub.
func waitService(w *world, root context.Context, pub publisher) (*Service, *countingPub) {
	cp := &countingPub{publisher: pub}
	return NewService(root, w.db, cp, testNow), cp
}

// waiting blocks until a Wait has subscribed through cp.
func waiting(t *testing.T, cp *countingPub) {
	t.Helper()
	deadline := time.After(waitTimeout)
	for cp.n.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("no Wait subscribed")
		case <-time.After(time.Millisecond):
		}
	}
}

// afterA2 waits for memberA's entries after a2.
func afterA2(w *world) WaitQuery { return WaitQuery{After: w.entries["a2"].ID, Timeout: longWait} }

func TestWaitReturnsPendingAtOnce(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	got, err := w.svc.Wait(t.Context(), w.principals["memberA"], WaitQuery{Timeout: longWait})
	if err != nil || !slices.Equal(bodies(got), []string{"a1", "a2"}) {
		t.Fatalf("Wait = %v, %v; want [a1 a2]", bodies(got), err)
	}
	if got := w.read(t, "memberA"); !slices.Equal(got, []string{"a1", "a2"}) {
		t.Fatalf("mailbox after Wait = %v, want the entries kept", got)
	}
}

func TestWaitBlocksAfterSeen(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	stillWaiting(t, startWait(t.Context(), w.svc, w.principals["memberA"], afterA2(w)))
}

func TestWaitEndsOnMatchingEntry(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	svc, cp := waitService(w, t.Context(), w.pub)
	ch := startWait(t.Context(), svc, w.principals["memberA"], afterA2(w))
	waiting(t, cp)
	w.send(t, "worker", "alpha", "a3")
	if got := result(t, ch); !slices.Equal(got, []string{"a3"}) {
		t.Fatalf("Wait = %v, want [a3]", got)
	}
}

func TestWaitIgnoresOtherSpace(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	svc, cp := waitService(w, t.Context(), w.pub)
	ch := startWait(t.Context(), svc, w.principals["memberA"], afterA2(w))
	waiting(t, cp)
	w.send(t, "worker", "beta", "b2")
	stillWaiting(t, ch)
}

// insertOnSubscribe stores an entry, without publishing it, right after a subscription
// is taken: the Wait must find it with its first read.
type insertOnSubscribe struct {
	publisher
	insert func()
}

func (p insertOnSubscribe) Subscribe(topic string) (<-chan []byte, func()) {
	ch, cancel := p.publisher.Subscribe(topic)
	p.insert()
	return ch, cancel
}

func TestWaitSubscribeBeforeRead(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	pub := insertOnSubscribe{publisher: w.pub, insert: func() { w.insert(t, "worker", "alpha", "a3") }}
	svc := NewService(t.Context(), w.db, pub, testNow)
	got, err := svc.Wait(t.Context(), w.principals["memberA"], afterA2(w))
	if err != nil || !slices.Equal(bodies(got), []string{"a3"}) {
		t.Fatalf("Wait = %v, %v; want [a3]", bodies(got), err)
	}
}

func TestWaitFilters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		filter func(q *WaitQuery)
		match  func(e *Entry)
	}{
		{"from", func(q *WaitQuery) { q.From = "lead" }, func(e *Entry) { e.From = "lead" }},
		{"threadId", func(q *WaitQuery) { q.ThreadID = 7 }, func(e *Entry) {
			e.Type, e.ThreadID, e.Blackboard = ThreadPost, 7, "b"
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			svc, cp := waitService(w, t.Context(), w.pub)
			q := afterA2(w)
			tc.filter(&q)
			ch := startWait(t.Context(), svc, w.principals["memberA"], q)
			waiting(t, cp)
			other := w.entry("worker", "alpha", "other")
			other.From = "peer"
			w.publishNew(t, other)
			stillWaiting(t, ch)
			match := w.entry("worker", "alpha", "match")
			tc.match(&match)
			w.publishNew(t, match)
			if got := result(t, ch); !slices.Equal(got, []string{"match"}) {
				t.Fatalf("Wait = %v, want [match]", got)
			}
		})
	}
}

// publishNew stores e and publishes it, as producers do after commit.
func (w *world) publishNew(t *testing.T, e Entry) {
	t.Helper()
	stored, err := w.svc.insert(t.Context(), w.db, []Entry{e})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	w.svc.publish(stored)
}

func TestWaitTimeout(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	q := afterA2(w)
	q.Timeout = time.Millisecond
	got, err := w.svc.Wait(t.Context(), w.principals["memberA"], q)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("Wait = %#v, %v; want an empty, non-nil list", got, err)
	}
}

func TestWaitNeverAcks(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	for range 2 {
		got, err := w.svc.Wait(t.Context(), w.principals["memberA"], WaitQuery{Timeout: longWait})
		if err != nil || !slices.Equal(bodies(got), []string{"a1", "a2"}) {
			t.Fatalf("Wait = %v, %v; want [a1 a2] each time", bodies(got), err)
		}
	}
}

func TestWaitBounds(t *testing.T) {
	t.Parallel()
	for _, seconds := range []int{0, 301} {
		_, err := WaitTimeout(&seconds)
		apperrtest.WantKind(t, err, apperr.Invalid, "timeoutSeconds must be 1..300")
	}
	if d, err := WaitTimeout(nil); err != nil || d != 30*time.Second {
		t.Fatalf("default timeout = %v, %v; want 30s", d, err)
	}
}

func TestWaitAdminScope(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	svc, cp := waitService(w, t.Context(), w.pub)
	ch := startWait(t.Context(), svc, w.principals["ops"], WaitQuery{Timeout: longWait})
	waiting(t, cp)
	w.send(t, "ops", "beta", "o1")
	if got := result(t, ch); !slices.Equal(got, []string{"o1"}) {
		t.Fatalf("admin Wait = %v, want [o1]", got)
	}
}

func TestWaitEndsOnDisconnect(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	svc, cp := waitService(w, t.Context(), w.pub)
	ctx, cancel := context.WithCancel(t.Context())
	ch := startWait(ctx, svc, w.principals["memberA"], afterA2(w))
	waiting(t, cp)
	cancel()
	result(t, ch)
	if n := cp.n.Load(); n != 0 {
		t.Fatalf("live subscriptions after disconnect = %d, want 0", n)
	}
}

func TestWaitEndsOnRoot(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	root, cancel := context.WithCancel(t.Context())
	svc, cp := waitService(w, root, w.pub)
	ch := startWait(t.Context(), svc, w.principals["memberA"], afterA2(w))
	waiting(t, cp)
	cancel()
	if got := result(t, ch); len(got) != 0 {
		t.Fatalf("Wait after shutdown = %v, want []", got)
	}
}

func TestWaitDroppedSubscriptionRereads(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	small := broker.NewMemory(1)
	svc, cp := waitService(w, t.Context(), small)
	q := afterA2(w)
	q.From = "lead"
	ch := startWait(t.Context(), svc, w.principals["memberA"], q)
	waiting(t, cp)
	match := w.entry("worker", "alpha", "a3")
	match.From = "lead"
	if _, err := svc.insert(t.Context(), w.db, []Entry{match}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	for range 3 { // overflow the one-slot buffer so the broker drops the subscription
		small.Publish(topic(w.agents["worker"]), nil)
	}
	if got := result(t, ch); !slices.Equal(got, []string{"a3"}) {
		t.Fatalf("Wait after a dropped subscription = %v, want [a3]", got)
	}
}

func TestWaitDoesNotTakeSlot(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.svc.heartbeat = time.Millisecond
	startStream(t.Context(), t, w, "memberA").entries(2)
	got, err := w.svc.Wait(t.Context(), w.principals["memberA"], WaitQuery{Timeout: longWait})
	if err != nil || len(got) != 2 {
		t.Fatalf("Wait beside an open stream = %v, %v", bodies(got), err)
	}
}
