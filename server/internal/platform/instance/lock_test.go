package instance

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestClaimIsExclusive: one instance per data dir; a second claim fails while the first holds
// the lock, whether or not the first has published its state yet.
func TestClaimIsExclusive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first := mustClaim(t, dir)
	_, err := claim(context.Background(), http.DefaultClient, dir, 0)
	if err == nil || err.Error() != "agent-message-bus already running (state pending)" {
		t.Fatalf("second claim before publishing: %v", err)
	}
	f := newFakeBus(t, dir, true)
	f.adopt(first)
	f.publish(t, 42)
	_, err = claim(context.Background(), http.DefaultClient, dir, 0)
	if err == nil || err.Error() != "agent-message-bus already running (pid 42) at "+f.srv.URL {
		t.Fatalf("second claim after publishing: %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	if err := mustClaim(t, dir).Release(); err != nil {
		t.Fatal(err)
	}
}

// TestClaimWaitsForProbe: a CLI probing the lock holds it for a moment; a claim retries until
// the probe is done instead of failing, and without patience it says the lock was busy.
func TestClaimWaitsForProbe(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	probe, err := lockFile(lockPath(dir), shared)
	if err != nil {
		t.Fatal(err)
	}
	_, err = claim(context.Background(), http.DefaultClient, dir, 0)
	if err == nil || err.Error() != "data dir lock "+lockPath(dir)+" was busy; retry" {
		t.Fatalf("claim during a probe without patience: %v", err)
	}
	pauses := 0
	f, err := lockExclusive(context.Background(), lockPath(dir), time.Hour, func() {
		pauses++
		if pauses == 1 {
			_ = probe.Close() // the probe is done
		}
	})
	if err != nil || pauses != 1 {
		t.Fatalf("lock after the probe: %v after %d pauses", err, pauses)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestClaimRemovesLeftoverState: a state file found by the claim is a crashed instance's.
func TestClaimRemovesLeftoverState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := writeState(dir, State{PID: 42, URL: "http://127.0.0.1:1", InstanceID: "crashed"}); err != nil {
		t.Fatal(err)
	}
	mustClaim(t, dir)
	if _, found, err := readState(dir); found || err != nil {
		t.Fatalf("leftover state found = %v, %v", found, err)
	}
}

// TestProbesShareTheLock: two CLIs probing at once both see a free lock.
func TestProbesShareTheLock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	probe, err := lockFile(lockPath(dir), shared)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = probe.Close() })
	inst, err := Find(context.Background(), http.DefaultClient, dir)
	if err != nil || inst.Running {
		t.Fatalf("Find during another probe = %+v, %v", inst, err)
	}
}

func TestPublishWritesStateAtomically(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	l := mustClaim(t, dir)
	unpublish, err := l.Publish("[::]:8080", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	st, found, err := readState(dir)
	want := State{PID: os.Getpid(), URL: "http://127.0.0.1:8080", StartedAt: "2026-01-02T03:04:05Z", InstanceID: l.ID()}
	if err != nil || !found || st != want || l.ID() == "" {
		t.Fatalf("got %+v %v %v, want %+v", st, found, err, want)
	}
	if files, err := os.ReadDir(dir); err != nil || len(files) != 2 {
		t.Fatalf("data dir holds %v (%v), want the lock and the state file", files, err)
	}
	if err := unpublish(); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := readState(dir); found {
		t.Fatal("state file still present")
	}
}

func mustClaim(t *testing.T, dir string) *Lock {
	t.Helper()
	l, err := claim(context.Background(), http.DefaultClient, dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Release() })
	return l
}

// TestClaimFailsFastOnServingInstance: a claim that finds the lock held by a serving instance
// fails at once; it never waits for that instance to stop, or it would take the data dir over
// from a start whose instance an operator just stopped.
func TestClaimFailsFastOnServingInstance(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first := mustClaim(t, dir)
	pauses := 0
	f, err := lockExclusive(context.Background(), lockPath(dir), time.Hour, func() {
		pauses++
		_ = first.Release() // the serving instance is stopped while the loser waits
	})
	if !errors.Is(err, errLocked) || pauses != 0 {
		t.Fatalf("lock held by a serving instance: %v after %d pauses", err, pauses)
	}
	if f != nil {
		_ = f.Close()
	}
	_ = first.Release()
}

// TestClaimLoserNeverTakesOver: the loser of a start race, with the full patience, still
// loses when the winner is stopped a moment later.
func TestClaimLoserNeverTakesOver(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	winner := mustClaim(t, dir)
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = winner.Release()
	}()
	l, err := claim(context.Background(), http.DefaultClient, dir, claimPatience)
	if err == nil {
		_ = l.Release()
		t.Fatal("the loser took the data dir over from the winner")
	}
}
