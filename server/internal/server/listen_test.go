package server

import (
	"net"
	"strings"
	"testing"
)

func TestListenWithoutGUI(t *testing.T) {
	t.Parallel()
	api, web, err := listen("127.0.0.1:0", "")
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
	api, web, err := listen("127.0.0.1:0", "127.0.0.1:0")
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
	_, _, err = listen(apiAddr, taken.Addr().String())
	if err == nil || !strings.Contains(err.Error(), "listen AMB_WEB_ADDR "+taken.Addr().String()) {
		t.Fatalf("err = %v, want a listen AMB_WEB_ADDR error", err)
	}
	again, err := net.Listen("tcp", apiAddr)
	if err != nil {
		t.Fatalf("API address still bound: %v", err)
	}
	_ = again.Close()
}
