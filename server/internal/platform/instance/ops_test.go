package instance

import (
	"context"
	"os"
	"strings"
	"testing"
)

// instanceKind sets up the data dir's instance for an operation test.
type instanceKind int

const (
	none    instanceKind = iota
	serving              // holds the lock, confirmed at its url as pid 7
	pending              // holds the lock, state file not written yet
)

func setUp(t *testing.T, m Manager, kind instanceKind, obey bool) *fakeBus {
	t.Helper()
	switch kind {
	case serving:
		return newRunningBus(t, m.dir(), obey, 7)
	case pending:
		f := newFakeBus(t, m.dir(), obey)
		f.adopt(mustClaim(t, m.dir()))
		return f
	}
	return newFakeBus(t, m.dir(), obey)
}

func TestStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		kind   instanceKind
		asJSON bool
		want   string
	}{
		{name: "text running", kind: serving, want: "running (pid 7) at http://"},
		{name: "text pending", kind: pending, want: "running (state pending)\n"},
		{name: "text stopped", want: "not running\n"},
		{name: "json running", kind: serving, asJSON: true, want: `{"running":true,"pid":7,"url":"http://`},
		{name: "json pending", kind: pending, asJSON: true, want: `{"running":true}` + "\n"},
		{name: "json stopped", asJSON: true, want: `{"running":false}` + "\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, out := newManager(t, "")
			setUp(t, m, tc.kind, true)
			if err := m.Status(context.Background(), tc.asJSON); err != nil {
				t.Fatal(err)
			}
			wantOut(t, out, tc.want)
		})
	}
}

func TestStart(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		kind          instanceKind
		spawnedPID    int // pid the spawned server publishes; 0: none becomes healthy
		wantErr, want string
	}{
		{name: "already running", kind: serving, want: "agent-message-bus already running (pid 7) at http://"},
		{name: "already running, pending", kind: pending, want: "agent-message-bus already running (state pending)\n"},
		{name: "started", spawnedPID: 9, want: "agent-message-bus started (pid 9) at http://"},
		{name: "another start won the race", spawnedPID: 5, want: "agent-message-bus already running (pid 5) at http://"},
		{name: "timeout", wantErr: "did not become healthy"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, out := newManager(t, "")
			f := setUp(t, m, tc.kind, true)
			m.Spawn = func(logPath string) (int, error) {
				_ = os.WriteFile(logPath, []byte("boom line\n"), 0o600)
				if tc.spawnedPID != 0 {
					f.adopt(mustClaim(t, m.dir()))
					f.publish(t, tc.spawnedPID)
				}
				return 9, nil
			}
			err := m.Start(context.Background())
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), "boom line") {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			wantOut(t, out, tc.want)
		})
	}
}

func TestStop(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		kind           instanceKind
		obey, wantKill bool
		want, wantErr  string
	}{
		{name: "not running", want: "agent-message-bus is not running\n"},
		{name: "graceful", kind: serving, obey: true, want: "agent-message-bus stopped (pid 7)\n"},
		{name: "kill fallback", kind: serving, wantKill: true, want: "agent-message-bus stopped (pid 7)\n"},
		{name: "pending is no target", kind: pending, wantErr: "agent-message-bus is running (state pending) but did not answer within 500ms"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, out := newManager(t, testToken)
			f := setUp(t, m, tc.kind, tc.obey)
			killed := 0
			m.Kill = func(pid int) error { killed = pid; f.crash(); return nil }
			err := m.Stop(context.Background())
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || len(f.recorded()) != 0 {
					t.Fatalf("err = %v, calls %q", err, f.recorded())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			wantOut(t, out, tc.want)
			if (killed == 7) != tc.wantKill {
				t.Fatalf("killed = %d", killed)
			}
			if _, err := os.Stat(statePath(m.dir())); !os.IsNotExist(err) {
				t.Fatalf("state file remains: %v", err)
			}
			if tc.kind == serving && f.recorded()[0] != "/v1/admin/shutdown Bearer "+testToken+" " {
				t.Fatalf("calls %q", f.recorded())
			}
		})
	}
}
