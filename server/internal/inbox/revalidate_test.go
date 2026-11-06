package inbox

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
)

// linesUntilClosed returns every line the stream sends until the server ends it.
func (r *sseReader) linesUntilClosed() []string {
	r.t.Helper()
	deadline := time.After(waitTimeout)
	var lines []string
	for {
		line, ok := r.nextLine(deadline)
		if !ok {
			return lines
		}
		lines = append(lines, line)
	}
}

// eventually fails t unless cond holds within waitTimeout.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(waitTimeout)
	for !cond() {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting until %s", what)
		case <-time.After(time.Millisecond):
		}
	}
}

// slotFreed waits until worker's stream slot is released.
func slotFreed(t *testing.T, w *world) {
	t.Helper()
	eventually(t, "the stream slot is free", func() bool { return !w.svc.Connected(w.agents["worker"]) })
}

func TestStreamEndsOnTheWakeUpAfterItsTokenStoppedAuthenticating(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.svc.heartbeat = time.Hour
	stream := startStream(t.Context(), t, w, "memberA")
	stream.entries(2)
	w.creds.revoke()
	w.send(t, "worker", "alpha", "secret for the next holder")
	if lines := stream.linesUntilClosed(); slices.ContainsFunc(lines, func(l string) bool { return strings.Contains(l, "secret") }) {
		t.Fatalf("the revoked stream got %v", lines)
	}
	slotFreed(t, w)
}

func TestIdleStreamEndsAtTheHeartbeatAfterItsTokenStoppedAuthenticating(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.svc.heartbeat = time.Millisecond
	stream := startStream(t.Context(), t, w, "memberA")
	stream.entries(2)
	stream.expectPing()
	w.creds.revoke()
	stream.expectClosed()
	slotFreed(t, w)
}

func TestWaitFailsOnTheWakeUpAfterItsTokenStoppedAuthenticating(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	svc, cp := waitService(w, t.Context(), w.pub)
	done := make(chan error, 1)
	go func() {
		got, err := svc.Wait(t.Context(), w.principals["memberA"], afterA2(w))
		if len(got) > 0 {
			t.Errorf("the revoked wait returned %v", bodies(got))
		}
		done <- err
	}()
	waiting(t, cp)
	w.creds.revoke()
	w.send(t, "worker", "alpha", "wait-leak")
	apperrtest.WantKind(t, <-done, apperr.Unauthenticated, "invalid token")
}

func TestIdleWaitFailsAtTheHeartbeatAfterItsTokenStoppedAuthenticating(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	svc, cp := waitService(w, t.Context(), w.pub)
	svc.heartbeat = time.Millisecond
	done := make(chan error, 1)
	go func() {
		_, err := svc.Wait(t.Context(), w.principals["memberA"], afterA2(w))
		done <- err
	}()
	waiting(t, cp)
	eventually(t, "the wait checked its token once", func() bool { return w.creds.checks.Load() >= 1 })
	w.creds.revoke() // only the heartbeat can notice it now
	select {
	case err := <-done:
		apperrtest.WantKind(t, err, apperr.Unauthenticated, "invalid token")
	case <-time.After(waitTimeout):
		t.Fatal("the revoked wait kept waiting")
	}
}

// The token check costs one query per wake-up and per page, not one per read: the read
// that finds nothing more after a page is covered by that page's check.
func TestStreamChecksItsTokenOncePerWakeUp(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.svc.heartbeat = time.Hour
	stream := startStream(t.Context(), t, w, "memberA")
	stream.entries(2)
	w.send(t, "worker", "alpha", "a3")
	stream.entries(1)
	w.send(t, "worker", "alpha", "a4")
	stream.entries(1)
	if got := w.creds.checks.Load(); got != 3 {
		t.Fatalf("token checks = %d, want 3 (backlog, a3, a4)", got)
	}
}

// revokeAndLeakAfterFirstCheck makes the first token check of w's stream or wait followed
// at once by a revocation and a new entry "leak" for memberA, as when revoke, re-invite and
// send commit between that check and the next read.
func revokeAndLeakAfterFirstCheck(t *testing.T, w *world) {
	w.creds.afterFirstCheck = func() {
		w.creds.revoke()
		es, err := w.svc.insert(t.Context(), w.db, []Entry{w.entry("worker", "alpha", "leak")})
		if err != nil {
			t.Errorf("Insert leak: %v", err)
			return
		}
		w.svc.publish(es)
	}
}

func TestStreamSendsNothingStoredAfterARevocationThatFollowsItsCheck(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.svc.heartbeat = time.Hour
	revokeAndLeakAfterFirstCheck(t, w)
	stream := startStream(t.Context(), t, w, "memberA")
	if lines := stream.linesUntilClosed(); slices.ContainsFunc(lines, func(l string) bool { return strings.Contains(l, "leak") }) {
		t.Fatalf("the revoked stream got %v", lines)
	}
	slotFreed(t, w)
}

func TestWaitReturnsNothingStoredAfterARevocationThatFollowsItsCheck(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	svc, _ := waitService(w, t.Context(), w.pub)
	svc.heartbeat = time.Hour
	revokeAndLeakAfterFirstCheck(t, w)
	got, err := svc.Wait(t.Context(), w.principals["memberA"], afterA2(w))
	if len(got) > 0 {
		t.Fatalf("the revoked wait returned %v", bodies(got))
	}
	apperrtest.WantKind(t, err, apperr.Unauthenticated, "invalid token")
}
