//go:build windows

package instance

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// errSharingViolation is ERROR_SHARING_VIOLATION: another handle's share mode denies the open.
const errSharingViolation = syscall.Errno(32)

// lockFile opens path, creating it, as a lock: the exclusive holder denies every other open
// (share mode 0), shared holders admit each other's reads but no exclusive holder.
func lockFile(path string, kind lockKind) (*os.File, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("lock file %s: %w", path, err)
	}
	share := uint32(syscall.FILE_SHARE_READ)
	if kind == exclusive {
		share = 0
	}
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ, share, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if errors.Is(err, errSharingViolation) {
		return nil, errLocked
	}
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}
