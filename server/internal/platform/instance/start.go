package instance

import (
	"context"
	"fmt"
	"os"
	"strings"
)

const logTailLines = 20

// Start launches a detached instance unless one is live and waits until it is healthy.
func (m Manager) Start(ctx context.Context) error {
	if st, live, err := Find(ctx, m.Client, m.dir()); err != nil || live {
		if live {
			_, _ = fmt.Fprintf(m.Out, "agent-message-bus already running (pid %d) at %s\n", st.PID, st.URL)
		}
		return err
	}
	if err := os.MkdirAll(m.dir(), 0o750); err != nil {
		return fmt.Errorf("creating data dir %s: %w", m.dir(), err)
	}
	if _, err := m.Spawn(m.logPath()); err != nil {
		return fmt.Errorf("spawning server: %w", err)
	}
	var st State
	ready := m.waitUntil(ctx, func() bool {
		s, live, _ := Find(ctx, m.Client, m.dir())
		st = s
		return live
	})
	if !ready {
		return fmt.Errorf("agent-message-bus did not become healthy within %s; log tail (%s):\n%s",
			m.Timeout, m.logPath(), logTail(m.logPath()))
	}
	_, _ = fmt.Fprintf(m.Out, "agent-message-bus started (pid %d) at %s; log: %s\n", st.PID, st.URL, m.logPath())
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
