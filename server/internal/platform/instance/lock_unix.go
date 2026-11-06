//go:build !windows

package instance

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// lockFile opens path, creating it, and flocks it without blocking: LOCK_EX for the exclusive
// holder, LOCK_SH for shared holders.
func lockFile(path string, kind lockKind) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	how := syscall.LOCK_SH
	if kind == exclusive {
		how = syscall.LOCK_EX
	}
	err = syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB)
	if err == nil {
		return f, nil
	}
	closeErr := f.Close()
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return nil, errors.Join(errLocked, closeErr)
	}
	return nil, errors.Join(fmt.Errorf("flock %s: %w", path, err), closeErr)
}
