package instance

import (
	"context"
	"fmt"
	"os"
	"strings"
)

const logTailLines = 20

// Start launches a detached instance unless one runs and waits until an instance serves the
// data dir.
func (m Manager) Start(ctx context.Context) error {
	inst, err := Find(ctx, m.Client, m.dir())
	if err != nil {
		return err
	}
	if inst.Running {
		_, _ = fmt.Fprintln(m.Out, alreadyRunning(inst))
		return nil
	}
	if err := os.MkdirAll(m.dir(), 0o750); err != nil {
		return fmt.Errorf("creating data dir %s: %w", m.dir(), err)
	}
	pid, err := m.Spawn(m.logPath())
	if err != nil {
		return fmt.Errorf("spawning server: %w", err)
	}
	return m.awaitStart(ctx, pid)
}

// awaitStart waits until an instance serves the data dir: the spawned pid, or another one that
// claimed the dir first, which the spawned server then leaves to it.
func (m Manager) awaitStart(ctx context.Context, pid int) error {
	inst, ok, err := m.watch(ctx, Instance.Serving)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("agent-message-bus did not become healthy within %s; log tail (%s):\n%s",
			m.Timeout, m.logPath(), logTail(m.logPath()))
	}
	if inst.State.PID != pid {
		_, _ = fmt.Fprintln(m.Out, alreadyRunning(inst))
		return nil
	}
	_, _ = fmt.Fprintf(m.Out, "agent-message-bus started (pid %d) at %s; log: %s\n", pid, inst.State.URL, m.logPath())
	return nil
}

func logTail(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("(unreadable: %v)", err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > logTailLines {
		lines = lines[len(lines)-logTailLines:]
	}
	return strings.Join(lines, "\n")
}
