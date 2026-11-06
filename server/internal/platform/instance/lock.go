package instance

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	lockFileName  = "agent-message-bus.lock"
	claimPatience = time.Second           // how long a claim rides out CLI probes
	claimRetry    = 10 * time.Millisecond // pause between claim attempts
)

// lockKind is how a process holds the data dir lock: a serving instance exclusively for its
// whole life, a CLI shared for the moment it probes.
type lockKind int

const (
	exclusive lockKind = iota
	shared
)

// errLocked reports that another process holds the lock in a conflicting kind.
var errLocked = errors.New("data dir lock is held")

// lockPath returns the lock file inside dir.
func lockPath(dir string) string { return filepath.Join(dir, lockFileName) }

// Lock is the serving instance's exclusive hold on its data dir: while it is held, no other
// instance serves the dir and the CLI counts the dir's instance as running. The OS releases it
// when the process ends, however it ends.
type Lock struct {
	dir, id string
	file    *os.File
}

// Claim takes dir's lock for the serving process, or fails with "agent-message-bus already
// running ..." while another instance holds it.
func Claim(ctx context.Context, client *http.Client, dir string) (*Lock, error) {
	return claim(ctx, client, dir, claimPatience)
}

func claim(ctx context.Context, client *http.Client, dir string, patience time.Duration) (*Lock, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("creating data dir %s: %w", dir, err)
	}
	f, err := lockExclusive(ctx, lockPath(dir), patience, pauseClaim)
	if errors.Is(err, errLocked) {
		return nil, claimedBy(ctx, client, dir)
	}
	if err != nil {
		return nil, fmt.Errorf("locking data dir: %w", err)
	}
	if err := removeState(dir); err != nil { // left by a crashed instance
		return nil, errors.Join(err, f.Close())
	}
	return &Lock{dir: dir, id: rand.Text(), file: f}, nil
}

func pauseClaim() { time.Sleep(claimRetry) }

// lockExclusive retries, calling pause in between, while CLI probes briefly share the lock,
// up to patience.
func lockExclusive(ctx context.Context, path string, patience time.Duration, pause func()) (*os.File, error) {
	deadline := time.Now().Add(patience)
	for {
		f, err := lockFile(path, exclusive)
		if !errors.Is(err, errLocked) || !time.Now().Before(deadline) || ctx.Err() != nil {
			return f, err
		}
		pause()
	}
}

// claimedBy describes the instance that holds dir's lock for a claim that failed.
func claimedBy(ctx context.Context, client *http.Client, dir string) error {
	inst, err := Find(ctx, client, dir)
	if err != nil {
		return err
	}
	if !inst.Running { // the holder let go just after the claim gave up
		return fmt.Errorf("data dir lock %s was busy; retry", lockPath(dir))
	}
	return errors.New(alreadyRunning(inst))
}

// ID is the random id of this instance, written to the state file and sent by /healthz.
func (l *Lock) ID() string { return l.id }

// Publish writes the state of the current process serving at addr and returns a func removing it.
func (l *Lock) Publish(addr string, now time.Time) (unpublish func() error, err error) {
	st := State{PID: os.Getpid(), URL: URLFor(addr), StartedAt: now.UTC().Format(time.RFC3339), InstanceID: l.id}
	if err := writeState(l.dir, st); err != nil {
		return nil, err
	}
	return func() error { return removeState(l.dir) }, nil
}

// Release gives the data dir up.
func (l *Lock) Release() error {
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("releasing data dir lock: %w", err)
	}
	return nil
}

// clearIfFree reports whether no instance holds dir's lock; then it removes the stale state
// file while its shared probe keeps instances from claiming the dir.
func clearIfFree(dir string) (free bool, err error) {
	f, err := lockFile(lockPath(dir), shared)
	switch {
	case errors.Is(err, errLocked):
		return false, nil
	case errors.Is(err, fs.ErrNotExist):
		return true, nil // no data dir, no instance
	case err != nil:
		return false, fmt.Errorf("probing data dir lock: %w", err)
	}
	return true, errors.Join(removeState(dir), f.Close())
}
