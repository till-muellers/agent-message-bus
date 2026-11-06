package server

import (
	"net"
	"runtime"
	"strings"
	"testing"
)

func TestListenWithoutGUI(t *testing.T) {
	t.Parallel()
	api, web, err := listen(t.Context(), "127.0.0.1:0", "")
	if err != nil {
		t.Fatal(err)
	}
	if web != nil {
		t.Fatalf("GUI listener %v, want none", web.Addr())
	}
	if err := closeListeners(api, web); err != nil {
		t.Fatal(err)
	}
}

func TestListenBoth(t *testing.T) {
	t.Parallel()
	api, web, err := listen(t.Context(), "127.0.0.1:0", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if web == nil || web.Addr().String() == api.Addr().String() {
		t.Fatalf("listeners api=%v web=%v, want two", api.Addr(), web)
	}
	if err := closeListeners(api, web); err != nil {
		t.Fatal(err)
	}
}

// TestListenGUIAddressTaken: when the GUI address is taken, the API listener is closed again.
func TestListenGUIAddressTaken(t *testing.T) {
	t.Parallel()
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = taken.Close() })
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	apiAddr := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}
	_, _, err = listen(t.Context(), apiAddr, taken.Addr().String())
	if err == nil || !strings.Contains(err.Error(), "listen AMB_WEB_ADDR "+taken.Addr().String()) {
		t.Fatalf("err = %v, want a listen AMB_WEB_ADDR error", err)
	}
	again, err := net.Listen("tcp", apiAddr)
	if err != nil {
		t.Fatalf("API address still bound: %v", err)
	}
	_ = again.Close()
}

// TestListenOverlappingAddresses: the GUI address may not overlap the API address on the same
// port, wildcard against loopback in either order. The second bind fails on Linux by default
// and on Windows with SO_EXCLUSIVEADDRUSE; BSD SO_REUSEADDR (macOS) still lets it through.
func TestListenOverlappingAddresses(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("overlapping binds are only refused on Windows and Linux so far")
	}
	for _, hosts := range [][2]string{{"", "127.0.0.1"}, {"0.0.0.0", "127.0.0.1"}, {"127.0.0.1", ""}, {"127.0.0.1", "0.0.0.0"}} {
		t.Run(hosts[0]+" vs "+hosts[1], func(t *testing.T) {
			t.Parallel()
			port := freePort(t)
			apiAddr, webAddr := net.JoinHostPort(hosts[0], port), net.JoinHostPort(hosts[1], port)
			api, web, err := listen(t.Context(), apiAddr, webAddr)
			if err == nil {
				_ = closeListeners(api, web)
				t.Fatalf("listen(%q, %q) bound both", apiAddr, webAddr)
			}
			if !strings.Contains(err.Error(), "listen AMB_WEB_ADDR "+webAddr) {
				t.Fatalf("err = %v, want a listen AMB_WEB_ADDR error", err)
			}
		})
	}
}

// freePort returns a TCP port that was free on loopback a moment ago.
func freePort(t *testing.T) string {
	t.Helper()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(probe.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}
