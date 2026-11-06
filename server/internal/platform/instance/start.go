package instance

import (
	"context"
	"fmt"
	"os"
	"strings"
)

const logTailLines = 20

// Start launches a detached instance unless one runs and waits until an instance serves the
// data dir. A running instance not yet confirmed at its url is waited for, not reported.
func (m Manager) Start(ctx context.Context) error {
	inst, err := m.settled(ctx)
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
		if ctx.Err() != nil {
			return fmt.Errorf("waiting for agent-message-bus to become healthy: %w", ctx.Err())
		}
		return m.unhealthy(inst, pid)
	}
	if inst.State.PID != pid {
		_, _ = fmt.Fprintln(m.Out, alreadyRunning(inst))
		return nil
	}
	_, _ = fmt.Fprintf(m.Out, "agent-message-bus started (pid %d) at %s; log: %s\n", pid, inst.State.URL, m.logPath())
	return nil
}

// unhealthy reports a start that timed out, after stopping the spawned server if it hangs.
func (m Manager) unhealthy(inst Instance, pid int) error {
	return fmt.Errorf("agent-message-bus did not become healthy within %s%s; log tail (%s):\n%s",
		m.Timeout, m.stopSpawned(inst, pid), m.logPath(), logTail(m.logPath()))
}

// stopSpawned kills the spawned pid while it holds the data dir unconfirmed, so no orphan keeps
// the port and the database, and says so. Only a lock holder whose state file names the pid is
// killed: a server that exited on its own, or one that has not published yet, is left alone.
func (m Manager) stopSpawned(inst Instance, pid int) string {
	if !inst.Running {
		return ""
	}
	st, found, err := readState(m.dir())
	if err == nil && (!found || st.PID != pid) {
		return ""
	}
	if err == nil {
		err = m.Kill(pid)
	}
	if err != nil {
		return fmt.Sprintf(" and could not be stopped: %v", err)
	}
	return fmt.Sprintf(" and was stopped (pid %d)", pid)
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
