package instance

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const healthTimeout = 2 * time.Second

// IDHeader is the /healthz response header that names the serving instance.
const IDHeader = "Agent-Bus-Instance"

// Instance is the instance of a data dir as the CLI sees it.
type Instance struct {
	Running bool  // a server holds the data dir lock
	State   State // its state once its url answers /healthz with its instance id; zero while pending
}

// Serving reports whether the instance runs and has been confirmed at its url.
func (i Instance) Serving() bool { return i.Running && i.State.URL != "" }

// Find reports the instance of dir. The lock decides whether one runs; the state file only
// names it. Without a lock holder any state file is stale and removed; with one, a state file
// counts once its url answers /healthz with its instance id, so a foreign instance on that url
// or a leftover file is never taken for it.
func Find(ctx context.Context, client *http.Client, dir string) (Instance, error) {
	free, err := clearIfFree(dir)
	if err != nil || free {
		return Instance{}, err
	}
	st, found, err := readState(dir)
	if err != nil {
		return Instance{}, err
	}
	if found && answers(ctx, client, st) {
		return Instance{Running: true, State: st}, nil
	}
	return Instance{Running: true}, nil
}

// answers reports whether st.URL/healthz answers "ok" with st's instance id.
func answers(ctx context.Context, client *http.Client, st State) bool {
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, st.URL+"/healthz", nil)
	if err != nil {
		return false
	}
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(res.Body, 16))
	return err == nil && res.StatusCode == http.StatusOK && string(b) == "ok" &&
		st.InstanceID != "" && res.Header.Get(IDHeader) == st.InstanceID
}

// alreadyRunning describes a running instance for start and for a second server.
func alreadyRunning(inst Instance) string {
	if !inst.Serving() {
		return "agent-message-bus already running (state pending)"
	}
	return fmt.Sprintf("agent-message-bus already running (pid %d) at %s", inst.State.PID, inst.State.URL)
}
