package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// finalWriteTimeout bounds the end of the response after the stream ended.
const finalWriteTimeout = time.Second

// sseWriter frames server-sent events and flushes each one to the client. Every frame
// write has a deadline of frameTimeout, so a client that stops reading fails the write;
// once done is closed, writes fail at once.
type sseWriter struct {
	w            io.Writer
	rc           *http.ResponseController
	frameTimeout time.Duration
	done         <-chan struct{}
}

func openSSE(w http.ResponseWriter, frameTimeout time.Duration, done <-chan struct{}) (sseWriter, error) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	sse := sseWriter{w: w, rc: http.NewResponseController(w), frameTimeout: frameTimeout, done: done}
	return sse, sse.flush()
}

func (s sseWriter) send(e Entry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encoding inbox entry %d: %w", e.ID, err)
	}
	if err := s.armDeadline(); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "id: %d\nevent: inbox\ndata: %s\n\n", e.ID, data); err != nil {
		return fmt.Errorf("writing inbox entry %d: %w", e.ID, err)
	}
	return s.flush()
}

func (s sseWriter) ping() error {
	if err := s.armDeadline(); err != nil {
		return err
	}
	if _, err := io.WriteString(s.w, ": ping\n\n"); err != nil {
		return fmt.Errorf("writing heartbeat: %w", err)
	}
	return s.flush()
}

// armDeadline gives the next frame frameTimeout to reach the client. It checks done after
// setting the deadline, so it never extends one that expire set meanwhile.
func (s sseWriter) armDeadline() error {
	if err := s.rc.SetWriteDeadline(time.Now().Add(s.frameTimeout)); err != nil {
		return fmt.Errorf("setting the frame write deadline: %w", err)
	}
	select {
	case <-s.done:
		return s.expire()
	default:
		return nil
	}
}

// expire makes every pending and later write fail at once, which ends a write blocked on
// a client that stopped reading.
func (s sseWriter) expire() error {
	if err := s.rc.SetWriteDeadline(time.Now()); err != nil {
		return fmt.Errorf("expiring the stream's write deadline: %w", err)
	}
	return nil
}

// expireWhenDone expires the writes once ctx ends (expire). The returned finish undoes
// that for the end of the response: it waits for an expire under way, then gives the
// final write finalWriteTimeout, so a reading client sees a clean end and a stalled one
// cannot hold the handler.
func (s sseWriter) expireWhenDone(ctx context.Context) (finish func() error) {
	fired := make(chan struct{})
	var expireErr error
	stop := context.AfterFunc(ctx, func() {
		defer close(fired)
		expireErr = s.expire()
	})
	return func() error {
		if !stop() {
			<-fired
		}
		if err := s.rc.SetWriteDeadline(time.Now().Add(finalWriteTimeout)); err != nil {
			return errors.Join(expireErr, fmt.Errorf("setting the final write deadline: %w", err))
		}
		return expireErr
	}
}

func (s sseWriter) flush() error {
	if err := s.rc.Flush(); err != nil {
		return fmt.Errorf("flushing inbox stream: %w", err)
	}
	return nil
}
