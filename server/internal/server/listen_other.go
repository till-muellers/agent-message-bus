//go:build !windows

package server

import "net"

// listenConfig is the default: on Linux an overlapping bind on the same port already fails
// with EADDRINUSE (macOS, with BSD SO_REUSEADDR semantics, still allows it).
func listenConfig() net.ListenConfig { return net.ListenConfig{} }
