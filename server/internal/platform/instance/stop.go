package instance

import (
	"context"
	"fmt"
	"os"
)

// Stop asks the live instance to shut down, kills it on timeout and removes the state file.
func (m Manager) Stop(ctx context.Context) error {
	st, live, err := Find(ctx, m.Client, m.dir())
	if err != nil {
		return err
	}
	if !live {
		_, _ = fmt.Fprintln(m.Out, "agent-message-bus is not running")
		return nil
	}
	if tok, err := m.Config.ResolveAdminToken(); err == nil {
		_ = m.adminPost(ctx, st.URL+"/v1/admin/shutdown", tok, "") // fall back to kill below
	}
	if !m.waitUntil(ctx, func() bool { return m.gone(ctx, st) }) {
		if err := m.Kill(st.PID); err != nil {
			return fmt.Errorf("killing pid %d: %w", st.PID, err)
		}
	}
	if err := RemoveState(m.dir()); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(m.Out, "agent-message-bus stopped (pid %d)\n", st.PID)
	return nil
}

// gone reports whether the instance removed its state file and stopped answering.
func (m Manager) gone(ctx context.Context, st State) bool {
	_, statErr := os.Stat(StatePath(m.dir()))
	return os.IsNotExist(statErr) && !Healthy(ctx, m.Client, st.URL)
}

// KillPID terminates pid.
func KillPID(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("finding pid %d: %w", pid, err)
	}
	if err := p.Kill(); err != nil {
		return fmt.Errorf("killing pid %d: %w", pid, err)
	}
	return nil
}
