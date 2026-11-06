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
		failures      int32 // /healthz probes the running instance fails first
		holds, down   bool  // the spawned server takes the data dir lock; it never answers /healthz
		spawnedPID    int   // pid in the state file the spawned server publishes; 0: none
		wantKill      bool  // start kills the spawned pid 9 on its timeout
		wantErr, want string
	}{
		{name: "already running", kind: serving, want: "agent-message-bus already running (pid 7) at http://"},
		{name: "running, confirmed while start waits", kind: serving, failures: 3,
			want: "agent-message-bus already running (pid 7) at http://"},
		{name: "running, pending for the whole timeout", kind: pending, wantErr: "is running (state pending)"},
		{name: "started", holds: true, spawnedPID: 9, want: "agent-message-bus started (pid 9) at http://"},
		{name: "another start won the race", holds: true, spawnedPID: 5, want: "agent-message-bus already running (pid 5) at http://"},
		{name: "timeout, the server exited", wantErr: "did not become healthy within 500ms; log tail"},
		{name: "timeout, a lock holder that has not published", holds: true,
			wantErr: "did not become healthy within 500ms; log tail"},
		{name: "timeout, the server published but doesn't answer", holds: true, down: true, spawnedPID: 9, wantKill: true,
			wantErr: "did not become healthy within 500ms and was stopped (pid 9); log tail"},
		{name: "timeout, another pid's unanswered state", holds: true, down: true, spawnedPID: 5,
			wantErr: "did not become healthy within 500ms; log tail"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, out := newManager(t, "")
			f := setUp(t, m, tc.kind, true)
			f.failures.Store(tc.failures)
			spawned, killed := false, 0
			m.Spawn = func(logPath string) (int, error) {
				spawned = true
				_ = os.WriteFile(logPath, []byte("boom line\n"), 0o600)
				spawn(t, f, tc.holds, tc.down, tc.spawnedPID)
				return 9, nil
			}
			m.Kill = func(pid int) error { killed = pid; f.crash(); return nil }
			err := m.Start(context.Background())
			if spawned != (tc.kind == none) || (killed == 9) != tc.wantKill {
				t.Fatalf("spawned = %v, killed %d", spawned, killed)
			}
			expectResult(t, err, tc.wantErr, out.String(), tc.want)
			if spawned && err != nil && !strings.Contains(err.Error(), "boom line") {
				t.Fatalf("failed start without the log tail: %v", err)
			}
		})
	}
}

// spawn plays the spawned server on f: it takes the data dir lock and publishes pid, or not.
func spawn(t *testing.T, f *fakeBus, holds, down bool, pid int) {
	t.Helper()
	if holds {
		f.adopt(mustClaim(t, f.dir))
	}
	if pid != 0 {
		f.publish(t, pid)
	}
	f.healthy.Store(!down)
}

func TestStop(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                     string
		kind                     instanceKind
		obey, takeover, wantKill bool
		want, wantErr            string
	}{
		{name: "not running", want: "agent-message-bus is not running\n"},
		{name: "graceful", kind: serving, obey: true, want: "agent-message-bus stopped (pid 7)\n"},
		{name: "kill fallback", kind: serving, wantKill: true, want: "agent-message-bus stopped (pid 7)\n"},
		{name: "pending is no target", kind: pending,
			wantErr: "agent-message-bus is running (state pending) but did not answer within 500ms; retry, or end the process that holds "},
		{name: "another instance took over: nothing is killed", kind: serving, takeover: true,
			wantErr: "but the state file no longer names pid 7; nothing was killed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, out := newManager(t, testToken)
			f := setUp(t, m, tc.kind, tc.obey)
			f.takeover = tc.takeover
			killed := 0
			m.Kill = func(pid int) error { killed = pid; f.crash(); return nil }
			expectResult(t, m.Stop(context.Background()), tc.wantErr, out.String(), tc.want)
			if (killed == 7) != tc.wantKill {
				t.Fatalf("killed = %d", killed)
			}
			expectShutdownCalls(t, f, tc.kind == serving)
			if _, err := os.Stat(statePath(m.dir())); tc.wantErr == "" && !os.IsNotExist(err) {
				t.Fatalf("state file remains: %v", err)
			}
		})
	}
}

// expectResult checks an operation's error, or else its output.
func expectResult(t *testing.T, err error, wantErr, out, want string) {
	t.Helper()
	if wantErr != "" {
		if err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("err = %v, want %q", err, wantErr)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, want) {
		t.Fatalf("output %q, want %q", out, want)
	}
}

func expectShutdownCalls(t *testing.T, f *fakeBus, want bool) {
	t.Helper()
	calls := f.recorded()
	if !want {
		if len(calls) != 0 {
			t.Fatalf("calls %q, want none", calls)
		}
		return
	}
	if len(calls) != 1 || calls[0] != "/v1/admin/shutdown Bearer "+testToken+" " {
		t.Fatalf("calls %q, want one shutdown", calls)
	}
}
