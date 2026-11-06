//go:build !windows

package server

import "net"

// listenConfig is the default: elsewhere an overlapping bind on the same port already fails
// with EADDRINUSE.
func listenConfig() net.ListenConfig { return net.ListenConfig{} }
