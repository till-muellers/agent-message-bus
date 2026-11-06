// Package instance manages the local bus instance of a data dir: lock, state file, liveness
// and the CLI operations.
package instance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const stateFileName = "agent-message-bus.state.json"

// State describes a serving instance.
type State struct {
	PID        int    `json:"pid"`
	URL        string `json:"url"`
	StartedAt  string `json:"startedAt"`
	InstanceID string `json:"instanceId"`
}

// statePath returns the state file inside dir.
func statePath(dir string) string { return filepath.Join(dir, stateFileName) }

// readState reads the state file; found is false when it is missing or unparseable (cut short
// by a crash), which the caller treats like no state at all.
func readState(dir string) (st State, found bool, err error) {
	b, err := os.ReadFile(statePath(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, fmt.Errorf("reading state file: %w", err)
	}
	if json.Unmarshal(b, &st) != nil {
		return State{}, false, nil
	}
	return st, true, nil
}

// writeState replaces the state file with st: it writes a temp file and renames it over the
// state file, so a reader never sees it half written.
func writeState(dir string, st State) error {
	b, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("encoding state: %w", err)
	}
	tmp := statePath(dir) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("writing state file: %w", err)
	}
	if err := os.Rename(tmp, statePath(dir)); err != nil {
		return errors.Join(fmt.Errorf("writing state file: %w", err), os.Remove(tmp))
	}
	return nil
}

// removeState deletes the state file; a missing file is not an error.
func removeState(dir string) error {
	if err := os.Remove(statePath(dir)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing state file: %w", err)
	}
	return nil
}
