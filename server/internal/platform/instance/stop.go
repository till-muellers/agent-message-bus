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
	if err := m.awaitExit(ctx, inst.State); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(m.Out, "agent-message-bus stopped (pid %d)\n", inst.State.PID)
	return nil
}

// awaitExit waits until the instance st released the data dir and kills it if it doesn't
// within Timeout.
func (m Manager) awaitExit(ctx context.Context, st State) error {
	if gone, err := m.released(ctx); err != nil || gone {
		return err
	}
	if err := m.ensureStillNamed(st); err != nil {
		return err
	}
	if err := m.Kill(st.PID); err != nil {
		return fmt.Errorf("killing pid %d: %w", st.PID, err)
	}
	gone, err := m.released(ctx)
	if err == nil && !gone {
		err = fmt.Errorf("agent-message-bus (pid %d) still holds %s after kill", st.PID, lockPath(m.dir()))
	}
	return err
}

// ensureStillNamed fails unless the state file still names instance st: once st removed it or
// another instance took the data dir over, the lock holder is no longer st's pid.
func (m Manager) ensureStillNamed(st State) error {
	now, found, err := readState(m.dir())
	if err != nil {
		return err
	}
	if !found || now.InstanceID != st.InstanceID {
		return fmt.Errorf("%s is still held after %s, but the state file no longer names pid %d; nothing was killed",
			lockPath(m.dir()), m.Timeout, st.PID)
	}
	return nil
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
