package instance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

var errSpawned = errors.New("spawned")

// staleOp is one CLI command run against a data dir whose instance is gone, with its output.
type staleOp struct {
	name, want string
	run        func(m Manager) error
}

// staleOps returns status, start (stopped right after it launched the server) and stop.
func staleOps() []staleOp {
	return []staleOp{
		{name: "status", want: "not running\n", run: func(m Manager) error { return m.Status(context.Background(), false) }},
		{name: "start", run: func(m Manager) error {
			m.Spawn = func(string) (int, error) { return 0, errSpawned }
			if err := m.Start(context.Background()); !errors.Is(err, errSpawned) {
				return err
			}
			return nil
		}},
		{name: "stop", want: "agent-message-bus is not running\n", run: func(m Manager) error { return m.Stop(context.Background()) }},
	}
}

// TestUnparseableStateIsStale: an empty or truncated state file of a gone instance blocks
// nothing: status says not running, start starts, stop has nothing to stop, and the file goes.
func TestUnparseableStateIsStale(t *testing.T) {
	t.Parallel()
	for _, content := range []string{"", "{x"} {
		for _, op := range staleOps() {
			t.Run(op.name+" "+content, func(t *testing.T) {
				t.Parallel()
				m, out := newManager(t, testToken)
				if err := os.WriteFile(statePath(m.dir()), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := op.run(m); err != nil {
					t.Fatalf("%s: %v", op.name, err)
				}
				expectStaleHandled(t, m, out.String(), op)
			})
		}
	}
}

// TestForeignInstanceOnStateURL: the instance of this data dir was killed hard and another
// data dir's instance now answers on its url; that instance is never reported, shut down or
// killed as this data dir's.
func TestForeignInstanceOnStateURL(t *testing.T) {
	t.Parallel()
	for _, op := range staleOps() {
		t.Run(op.name, func(t *testing.T) {
			t.Parallel()
			m, out := newManager(t, testToken)
			foreign := newFakeBus(t, t.TempDir(), true)
			if err := writeState(m.dir(), State{PID: 42, URL: foreign.srv.URL, StartedAt: "2026-01-01T00:00:00Z"}); err != nil {
				t.Fatal(err)
			}
			if err := op.run(m); err != nil {
				t.Fatalf("%s: %v", op.name, err)
			}
			expectStaleHandled(t, m, out.String(), op)
			if calls := foreign.recorded(); len(calls) != 0 {
				t.Fatalf("the foreign instance got %q", calls)
			}
		})
	}
}

// TestMissingDataDir: status and stop on a data dir that doesn't exist report no instance and
// create nothing.
func TestMissingDataDir(t *testing.T) {
	t.Parallel()
	for _, op := range staleOps() {
		if op.name == "start" {
			continue
		}
		t.Run(op.name, func(t *testing.T) {
			t.Parallel()
			m, out := newManager(t, testToken)
			m.Config.DBPath = filepath.Join(m.dir(), "missing", "bus.db")
			if err := op.run(m); err != nil || out.String() != op.want {
				t.Fatalf("%s: %v, printed %q", op.name, err, out)
			}
			if _, err := os.Stat(m.dir()); !os.IsNotExist(err) {
				t.Fatalf("%s created the data dir: %v", op.name, err)
			}
		})
	}
}

func expectStaleHandled(t *testing.T, m Manager, out string, op staleOp) {
	t.Helper()
	if out != op.want {
		t.Fatalf("%s printed %q, want %q", op.name, out, op.want)
	}
	if _, err := os.Stat(statePath(m.dir())); !os.IsNotExist(err) {
		t.Fatalf("stale state file remains after %s: %v", op.name, err)
	}
}
