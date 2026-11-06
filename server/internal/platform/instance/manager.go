package instance

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/config"
)

const logFileName = "agent-message-bus.log"

// Manager runs the CLI operations against the instance of Config's data dir.
type Manager struct {
	Config  config.Config
	Client  *http.Client
	Spawn   func(logPath string) (pid int, err error) // launches the detached server
	Kill    func(pid int) error
	Out     io.Writer
	Timeout time.Duration // how long start/stop wait
	Poll    time.Duration
}

func (m Manager) dir() string     { return m.Config.DataDir() }
func (m Manager) logPath() string { return filepath.Join(m.dir(), logFileName) }

// waitUntil polls cond every Poll until it holds or Timeout elapses.
func (m Manager) waitUntil(ctx context.Context, cond func() bool) bool {
	deadline := time.Now().Add(m.Timeout)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return false
		}
		time.Sleep(m.Poll)
	}
}

// watch polls Find until done holds for the instance, Find fails or Timeout elapses; ok is
// false on the timeout.
func (m Manager) watch(ctx context.Context, done func(Instance) bool) (inst Instance, ok bool, err error) {
	ok = m.waitUntil(ctx, func() bool {
		inst, err = Find(ctx, m.Client, m.dir())
		return err != nil || done(inst)
	})
	return inst, ok, err
}

// settled returns the instance once it is either serving or not running; an instance that
// holds the lock but stays unconfirmed at its url for Timeout is an error, never a target.
func (m Manager) settled(ctx context.Context) (Instance, error) {
	inst, ok, err := m.watch(ctx, func(i Instance) bool { return !i.Running || i.Serving() })
	if err != nil {
		return Instance{}, err
	}
	if !ok {
		return Instance{}, fmt.Errorf("agent-message-bus is running (state pending) but did not answer within %s; "+
			"retry, or end the process that holds %s", m.Timeout, lockPath(m.dir()))
	}
	return inst, nil
}
