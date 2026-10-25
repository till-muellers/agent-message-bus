package instance

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const healthTimeout = 2 * time.Second

// Healthy reports whether url/healthz answers "ok".
func Healthy(ctx context.Context, client *http.Client, url string) bool {
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/healthz", nil)
	if err != nil {
		return false
	}
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(res.Body, 16))
	return err == nil && res.StatusCode == http.StatusOK && string(b) == "ok"
}

// Find returns the live instance of dir. A stale state file (healthz failing) is removed.
func Find(ctx context.Context, client *http.Client, dir string) (State, bool, error) {
	st, found, err := ReadState(dir)
	if err != nil || !found {
		return State{}, false, err
	}
	if Healthy(ctx, client, st.URL) {
		return st, true, nil
	}
	if err := RemoveState(dir); err != nil {
		return State{}, false, fmt.Errorf("removing stale state: %w", err)
	}
	return State{}, false, nil
}

// EnsureNotRunning fails when dir already has a live instance.
func EnsureNotRunning(ctx context.Context, client *http.Client, dir string) error {
	st, live, err := Find(ctx, client, dir)
	if err != nil {
		return err
	}
	if live {
		return fmt.Errorf("agent-message-bus already running (pid %d) at %s", st.PID, st.URL)
	}
	return nil
}
