package instance

import (
	"context"
	"errors"
	"os"
	"testing"
)

var errSpawned = errors.New("spawned")

// staleOps runs one CLI command against a data dir whose instance is gone; spawn stops start
// right after it launched the server.
var staleOps = map[string]func(m Manager) error{
	"status": func(m Manager) error { return m.Status(context.Background(), false) },
	"start": func(m Manager) error {
		m.Spawn = func(string) (int, error) { return 0, errSpawned }
		if err := m.Start(context.Background()); !errors.Is(err, errSpawned) {
			return err
		}
		return nil
	},
	"stop": func(m Manager) error { return m.Stop(context.Background()) },
}

var staleOutput = map[string]string{
	"status": "not running\n",
	"start":  "",
	"stop":   "agent-message-bus is not running\n",
}

// TestUnparseableStateIsStale: an empty or truncated state file of a gone instance blocks
// nothing: status says not running, start starts, stop has nothing to stop, and the file goes.
func TestUnparseableStateIsStale(t *testing.T) {
	t.Parallel()
	for _, content := range []string{"", "{x"} {
		for op, run := range staleOps {
			t.Run(op+" "+content, func(t *testing.T) {
				t.Parallel()
				m, out := newManager(t, testToken)
				if err := os.WriteFile(statePath(m.dir()), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := run(m); err != nil {
					t.Fatalf("%s: %v", op, err)
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
	for op, run := range staleOps {
		t.Run(op, func(t *testing.T) {
			t.Parallel()
			m, out := newManager(t, testToken)
			foreign := newFakeBus(t, t.TempDir(), true)
			if err := writeState(m.dir(), State{PID: 42, URL: foreign.srv.URL, StartedAt: "2026-01-01T00:00:00Z"}); err != nil {
				t.Fatal(err)
			}
			if err := run(m); err != nil {
				t.Fatalf("%s: %v", op, err)
			}
			expectStaleHandled(t, m, out.String(), op)
			if calls := foreign.recorded(); len(calls) != 0 {
				t.Fatalf("the foreign instance got %q", calls)
			}
		})
	}
}

func expectStaleHandled(t *testing.T, m Manager, out, op string) {
	t.Helper()
	if out != staleOutput[op] {
		t.Fatalf("%s printed %q, want %q", op, out, staleOutput[op])
	}
	if _, err := os.Stat(statePath(m.dir())); !os.IsNotExist(err) {
		t.Fatalf("stale state file remains after %s: %v", op, err)
	}
}
