package instance

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestURLFor(t *testing.T) {
	t.Parallel()
	tests := []struct{ addr, want string }{
		{"127.0.0.1:8080", "http://127.0.0.1:8080"},
		{"[::]:9", "http://127.0.0.1:9"},
		{"0.0.0.0:9", "http://127.0.0.1:9"},
		{":9", "http://127.0.0.1:9"},
		{"[::1]:9", "http://[::1]:9"},
		{"localhost:9", "http://localhost:9"},
	}
	for _, tc := range tests {
		if got := URLFor(tc.addr); got != tc.want {
			t.Fatalf("URLFor(%q) = %q, want %q", tc.addr, got, tc.want)
		}
	}
}

func TestPublish(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cleanup, err := Publish(dir, "[::]:8080", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	st, found, err := ReadState(dir)
	want := State{PID: os.Getpid(), URL: "http://127.0.0.1:8080", StartedAt: "2026-01-02T03:04:05Z"}
	if err != nil || !found || st != want {
		t.Fatalf("got %+v %v %v", st, found, err)
	}
	cleanup()
	if _, err := os.Stat(StatePath(dir)); !os.IsNotExist(err) {
		t.Fatalf("state file still present: %v", err)
	}
}

func TestFind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		publish, healthy   bool
		wantLive, wantFile bool
	}{
		{name: "no state"},
		{name: "live", publish: true, healthy: true, wantLive: true, wantFile: true},
		{name: "stale", publish: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			f := newFakeBus(t, dir, true)
			f.healthy.Store(tc.healthy)
			if tc.publish {
				f.publish(t, 42)
			}
			err := EnsureNotRunning(context.Background(), http.DefaultClient, dir)
			if (err != nil) != tc.wantLive {
				t.Fatalf("EnsureNotRunning err = %v", err)
			}
			if tc.wantLive && err.Error() != "agent-message-bus already running (pid 42) at "+f.srv.URL {
				t.Fatalf("err = %v", err)
			}
			_, statErr := os.Stat(StatePath(dir))
			if (statErr == nil) != tc.wantFile {
				t.Fatalf("state file present = %v, want %v", statErr == nil, tc.wantFile)
			}
		})
	}
}
