package instance

import (
	"context"
	"errors"
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
	if err := m.requestShutdown(ctx, inst.State.URL); err != nil {
		return err
	}
	if err := m.awaitExit(ctx, inst.State); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(m.Out, "agent-message-bus stopped (pid %d)\n", inst.State.PID)
	return nil
}

// requestShutdown asks the instance at url to shut down. A server that refuses the request is
// healthy, not stuck: that is an error and nothing is killed. Any other failure is left to
// awaitExit, which kills an instance that does not leave.
func (m Manager) requestShutdown(ctx context.Context, url string) error {
	tok, err := m.Config.ResolveAdminToken()
	if err != nil {
		return nil // the instance is stopped by awaitExit
	}
	var refused *statusError
	if err := m.adminPost(ctx, url+"/v1/admin/shutdown", tok, ""); errors.As(err, &refused) && refused.rejected() {
		return fmt.Errorf("asking the instance to shut down: %w; the admin token in %s is not the admin token of the running instance, nothing was stopped",
			err, m.tokenSource())
	}
	return nil
}

// tokenSource names where the admin token of this CLI comes from.
func (m Manager) tokenSource() string {
	if m.Config.AdminToken != "" {
		return "AMB_ADMIN_TOKEN"
	}
	return m.Config.TokenPath()
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
