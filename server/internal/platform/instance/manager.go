package instance

import (
	"context"
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
