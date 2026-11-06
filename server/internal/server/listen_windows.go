//go:build windows

package server

import (
	"errors"
	"fmt"
	"net"
	"syscall"
)

// soExclusiveAddrUse is SO_EXCLUSIVEADDRUSE (~SO_REUSEADDR in winsock2.h).
const soExclusiveAddrUse = ^syscall.SO_REUSEADDR

// listenConfig binds with SO_EXCLUSIVEADDRUSE: by default Windows lets a specific-address
// bind (127.0.0.1:P) sit next to a wildcard bind (:P) on the same port, in either order, and
// routes loopback connections to the more specific socket.
func listenConfig() net.ListenConfig {
	return net.ListenConfig{Control: exclusiveAddrUse}
}

func exclusiveAddrUse(_, address string, c syscall.RawConn) error {
	var sockErr error
	err := c.Control(func(fd uintptr) {
		sockErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, soExclusiveAddrUse, 1)
	})
	if err = errors.Join(err, sockErr); err != nil {
		return fmt.Errorf("setting SO_EXCLUSIVEADDRUSE on %s: %w", address, err)
	}
	return nil
}
