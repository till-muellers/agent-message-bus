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

// Status prints whether an instance is live, as text or JSON.
func (m Manager) Status(ctx context.Context, asJSON bool) error {
	st, live, err := Find(ctx, m.Client, m.dir())
	if err != nil {
		return err
	}
	if asJSON {
		out := statusJSON{Running: live, PID: st.PID, URL: st.URL}
		if err := json.NewEncoder(m.Out).Encode(out); err != nil {
			return fmt.Errorf("writing status: %w", err)
		}
		return nil
	}
	if live {
		_, _ = fmt.Fprintf(m.Out, "running (pid %d) at %s\n", st.PID, st.URL)
	} else {
		_, _ = fmt.Fprintln(m.Out, "not running")
	}
	return nil
}
