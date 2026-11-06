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
// that finds nothing more after a page is covered by that page's check. Whether the
// entry sent after the revocation is found by that read or by its own wake-up, the check
// that ends the stream is the second.
func TestStreamChecksItsTokenOncePerWakeUp(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.svc.heartbeat = time.Hour
	stream := startStream(t.Context(), t, w, "memberA")
	stream.entries(2)
	w.creds.revoke()
	w.send(t, "worker", "alpha", "a3")
	stream.expectClosed()
	if got := w.creds.checks.Load(); got != 2 {
		t.Fatalf("token checks = %d, want 2 (the backlog page, the wake-up for a3)", got)
	}
}

// revokeAndStoreAfterFirstCheck makes the first token check of w's stream or wait followed
// at once by a revocation and a new entry body for worker in space, as when revoke,
// re-invite and send commit between that check and the next read.
func revokeAndStoreAfterFirstCheck(t *testing.T, w *world, space, body string) {
	w.creds.afterFirstCheck = func() {
		w.creds.revoke()
		es, err := w.svc.insert(t.Context(), w.db, []Entry{w.entry("worker", space, body)})
		if err != nil {
			t.Errorf("Insert %s: %v", body, err)
			return
		}
		w.svc.publish(es)
	}
}

func TestStreamSendsNothingStoredAfterARevocationThatFollowsItsCheck(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.svc.heartbeat = time.Hour
	revokeAndStoreAfterFirstCheck(t, w, "alpha", "leak")
	stream := startStream(t.Context(), t, w, "memberA")
	if lines := stream.linesUntilClosed(); slices.ContainsFunc(lines, func(l string) bool { return strings.Contains(l, "leak") }) {
		t.Fatalf("the revoked stream got %v", lines)
	}
	slotFreed(t, w)
}

// A wake-up that arrives while a page goes out is discarded, since the next read covers
// it; that read must still check the token even when it finds nothing in the stream's
// scope, as for mail of a space created again under a new id.
func TestStreamWokenDuringAPageChecksItsTokenEvenWithNothingToSend(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.svc.heartbeat = time.Hour
	revokeAndStoreAfterFirstCheck(t, w, "beta", "other scope")
	stream := startStream(t.Context(), t, w, "memberA")
	stream.entries(2)
	stream.expectClosed()
	slotFreed(t, w)
}

func TestWaitReturnsNothingStoredAfterARevocationThatFollowsItsCheck(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	svc, _ := waitService(w, t.Context(), w.pub)
	svc.heartbeat = time.Hour
	revokeAndStoreAfterFirstCheck(t, w, "alpha", "leak")
	got, err := svc.Wait(t.Context(), w.principals["memberA"], afterA2(w))
	if len(got) > 0 {
		t.Fatalf("the revoked wait returned %v", bodies(got))
	}
	apperrtest.WantKind(t, err, apperr.Unauthenticated, "invalid token")
}

func TestWakeEndsAnIdleStreamOfADeadToken(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.svc.heartbeat = time.Hour
	stream := startStream(t.Context(), t, w, "memberA")
	stream.entries(2)
	w.creds.revoke()
	w.svc.Wake(w.principals["memberA"].AgentID)
	stream.expectClosed()
	slotFreed(t, w)
}

func TestWakeFailsAnIdleWaitOfADeadToken(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	svc, cp := waitService(w, t.Context(), w.pub)
	svc.heartbeat = time.Hour
	done := make(chan error, 1)
	go func() {
		_, err := svc.Wait(t.Context(), w.principals["memberA"], afterA2(w))
		done <- err
	}()
	waiting(t, cp)
	w.creds.revoke()
	svc.Wake(w.principals["memberA"].AgentID)
	select {
	case err := <-done:
		apperrtest.WantKind(t, err, apperr.Unauthenticated, "invalid token")
	case <-time.After(waitTimeout):
		t.Fatal("the woken wait kept waiting")
	}
}
