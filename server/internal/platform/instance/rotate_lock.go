package instance

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// rotateLockPath is the lock file that serializes rotate-token runs on one data dir.
func (m Manager) rotateLockPath() string { return m.Config.TokenPath() + ".lock" }

// lockRotation waits up to Timeout until no other rotate-token run holds the data dir's
// rotation lock and takes it, so a run's swap on the instance and its write of the token file
// are not interleaved with another run's. The returned func releases it.
func (m Manager) lockRotation(ctx context.Context) (release func(), err error) {
	path := m.rotateLockPath()
	if err := os.MkdirAll(m.dir(), 0o750); err != nil {
		return nil, fmt.Errorf("creating data dir %s: %w", m.dir(), err)
	}
	var f *os.File
	m.waitUntil(ctx, func() bool {
		f, err = lockFile(path, exclusive)
		return !errors.Is(err, errLocked)
	})
	switch {
	case errors.Is(err, errLocked):
		return nil, fmt.Errorf("another rotate-token still runs (%s is held after %s)", path, m.Timeout)
	case err != nil:
		return nil, fmt.Errorf("locking %s: %w", path, err)
	}
	return func() { _ = f.Close() }, nil
}
