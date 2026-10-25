// Package instance manages the local bus instance of a data dir: state file, liveness and the CLI operations.
package instance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const stateFileName = "agent-message-bus.state.json"

// State describes a serving instance.
type State struct {
	PID       int    `json:"pid"`
	URL       string `json:"url"`
	StartedAt string `json:"startedAt"`
}

// StatePath returns the state file inside dir.
func StatePath(dir string) string { return filepath.Join(dir, stateFileName) }

// ReadState reads the state file; found is false when it does not exist.
func ReadState(dir string) (st State, found bool, err error) {
	b, err := os.ReadFile(StatePath(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, fmt.Errorf("reading state file: %w", err)
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return State{}, false, fmt.Errorf("parsing state file %s: %w", StatePath(dir), err)
	}
	return st, true, nil
}

// WriteState writes st to the state file.
func WriteState(dir string, st State) error {
	b, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("encoding state: %w", err)
	}
	if err := os.WriteFile(StatePath(dir), b, 0o600); err != nil {
		return fmt.Errorf("writing state file: %w", err)
	}
	return nil
}

// RemoveState deletes the state file; a missing file is not an error.
func RemoveState(dir string) error {
	if err := os.Remove(StatePath(dir)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing state file: %w", err)
	}
	return nil
}

// Publish writes the state of the current process serving at addr and returns a func removing it.
func Publish(dir, addr string, now time.Time) (func(), error) {
	st := State{PID: os.Getpid(), URL: URLFor(addr), StartedAt: now.UTC().Format(time.RFC3339)}
	if err := WriteState(dir, st); err != nil {
		return nil, err
	}
	return func() { _ = RemoveState(dir) }, nil
}
