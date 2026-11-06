package instance

import (
	"context"
	"encoding/json"
	"fmt"
)

type statusJSON struct {
	Running bool   `json:"running"`
	PID     int    `json:"pid,omitempty"`
	URL     string `json:"url,omitempty"`
}

// Status prints whether an instance runs, as text or JSON; pid and url only once it is
// confirmed at its url.
func (m Manager) Status(ctx context.Context, asJSON bool) error {
	inst, err := Find(ctx, m.Client, m.dir())
	if err != nil {
		return err
	}
	if asJSON {
		out := statusJSON{Running: inst.Running, PID: inst.State.PID, URL: inst.State.URL}
		if err := json.NewEncoder(m.Out).Encode(out); err != nil {
			return fmt.Errorf("writing status: %w", err)
		}
		return nil
	}
	switch {
	case inst.Serving():
		_, _ = fmt.Fprintf(m.Out, "running (pid %d) at %s\n", inst.State.PID, inst.State.URL)
	case inst.Running:
		_, _ = fmt.Fprintln(m.Out, "running (state pending)")
	default:
		_, _ = fmt.Fprintln(m.Out, "not running")
	}
	return nil
}
