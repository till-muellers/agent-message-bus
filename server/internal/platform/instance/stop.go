package instance

import (
	"context"
	"fmt"
	"os"
)

// Stop asks this data dir's instance to shut down, kills it when it still holds the data dir
// after Timeout and waits until it let go. Only an instance confirmed at its url is a target.
func (m Manager) Stop(ctx context.Context) error {
	inst, err := m.settled(ctx)
	if err != nil {
		return err
	}
	if !inst.Running {
		_, _ = fmt.Fprintln(m.Out, "agent-message-bus is not running")
		return nil
	}
	if tok, err := m.Config.ResolveAdminToken(); err == nil {
		_ = m.adminPost(ctx, inst.State.URL+"/v1/admin/shutdown", tok, "") // fall back to kill below
	}
	if err := m.awaitExit(ctx, inst.State.PID); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(m.Out, "agent-message-bus stopped (pid %d)\n", inst.State.PID)
	return nil
}

// awaitExit waits until the instance with pid released the data dir and kills it if it
// doesn't within Timeout.
func (m Manager) awaitExit(ctx context.Context, pid int) error {
	if gone, err := m.released(ctx); err != nil || gone {
		return err
	}
	if err := m.Kill(pid); err != nil {
		return fmt.Errorf("killing pid %d: %w", pid, err)
	}
	gone, err := m.released(ctx)
	if err == nil && !gone {
		err = fmt.Errorf("agent-message-bus (pid %d) still holds %s after kill", pid, lockPath(m.dir()))
	}
	return err
}

// released waits up to Timeout until no instance holds the data dir lock; the stale state file
// is removed then.
func (m Manager) released(ctx context.Context) (bool, error) {
	var err error
	gone := m.waitUntil(ctx, func() bool {
		var free bool
		free, err = clearIfFree(m.dir())
		return err != nil || free
	})
	return gone && err == nil, err
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
