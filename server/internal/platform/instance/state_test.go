package instance

import (
	"context"
	"net/http"
	"os"
	"testing"
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

// TestFind: the lock decides whether an instance runs; a state file only names it once its url
// confirms the instance id. A running instance's state file is never removed.
func TestFind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                   string
		locked, publish        bool
		healthy, foreign       bool
		state                  string // raw state file content instead of a published state
		wantRunning, wantState bool
		wantFile               bool
	}{
		{name: "no state"},
		{name: "serving", locked: true, publish: true, healthy: true, wantRunning: true, wantState: true, wantFile: true},
		{name: "stale", publish: true, healthy: true},
		{name: "stale and unparseable", state: "{x"},
		{name: "running, not healthy yet", locked: true, publish: true, wantRunning: true, wantFile: true},
		{name: "running, url answered by another instance", locked: true, publish: true, healthy: true, foreign: true,
			wantRunning: true, wantFile: true},
		{name: "running, state unparseable", locked: true, state: "{x", wantRunning: true, wantFile: true},
		{name: "running, state pending", locked: true, wantRunning: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			f := newFakeBus(t, dir, true)
			if tc.locked {
				f.adopt(mustClaim(t, dir))
			}
			f.healthy.Store(tc.healthy)
			findSetUp(t, f, tc.publish, tc.foreign, tc.state)
			inst, err := Find(context.Background(), http.DefaultClient, dir)
			if err != nil || inst.Running != tc.wantRunning || (inst.State.PID == 42) != tc.wantState {
				t.Fatalf("Find = %+v, %v", inst, err)
			}
			if _, statErr := os.Stat(statePath(dir)); (statErr == nil) != tc.wantFile {
				t.Fatalf("state file present = %v, want %v", statErr == nil, tc.wantFile)
			}
		})
	}
}

func findSetUp(t *testing.T, f *fakeBus, publish, foreign bool, state string) {
	t.Helper()
	if publish {
		f.publish(t, 42)
	}
	if foreign {
		f.mu.Lock()
		f.id = "foreign-instance"
		f.mu.Unlock()
	}
	if state != "" {
		if err := os.WriteFile(statePath(f.dir), []byte(state), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
