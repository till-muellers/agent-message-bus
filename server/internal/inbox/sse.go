package inbox

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// sseWriter frames server-sent events and flushes each one to the client.
type sseWriter struct {
	w  io.Writer
	rc *http.ResponseController
}

func openSSE(w http.ResponseWriter) (sseWriter, error) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	sse := sseWriter{w: w, rc: http.NewResponseController(w)}
	return sse, sse.flush()
}

func (s sseWriter) send(e Entry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encoding inbox entry %d: %w", e.ID, err)
	}
	if _, err := fmt.Fprintf(s.w, "id: %d\nevent: inbox\ndata: %s\n\n", e.ID, data); err != nil {
		return fmt.Errorf("writing inbox entry %d: %w", e.ID, err)
	}
	return s.flush()
}

func (s sseWriter) ping() error {
	if _, err := io.WriteString(s.w, ": ping\n\n"); err != nil {
		return fmt.Errorf("writing heartbeat: %w", err)
	}
	return s.flush()
}

func (s sseWriter) flush() error {
	if err := s.rc.Flush(); err != nil {
		return fmt.Errorf("flushing inbox stream: %w", err)
	}
	return nil
}
