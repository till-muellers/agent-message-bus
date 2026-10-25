package inbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpx"
)

// StreamHandler serves GET /v1/inbox/stream as server-sent events: the stored
// backlog, then live entries, until the request, root or subscription ends.
func (s *Service) StreamHandler(root context.Context) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := streamPrincipal(r.Context())
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		stop := context.AfterFunc(root, cancel)
		defer stop()
		if err := s.stream(ctx, w, p); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "inbox stream failed", "agent_id", p.AgentID, "err", err)
		}
	})
}

func streamPrincipal(ctx context.Context) (auth.Principal, error) {
	p, ok := auth.FromContext(ctx)
	if !ok {
		return auth.Principal{}, apperr.New(apperr.Unauthenticated, "missing bearer token")
	}
	return p, requireAgent(p)
}

// stream subscribes before draining the backlog so nothing published meanwhile
// is missed. Live messages are only wake-up signals: each one drains the inbox
// after lastSent, so entries go out once and in id order even when producers
// publish out of order.
func (s *Service) stream(ctx context.Context, w http.ResponseWriter, p auth.Principal) error {
	live, unsubscribe := s.pub.Subscribe(topic(p.AgentID))
	defer unsubscribe()
	sse, err := openSSE(w)
	if err != nil {
		return err
	}
	lastSent, err := s.drain(ctx, sse, p, 0)
	if err != nil {
		return err
	}
	return s.forwardLive(ctx, sse, p, live, lastSent)
}

// drain sends p's stored entries after lastSent page by page and returns the last id sent.
func (s *Service) drain(ctx context.Context, sse sseWriter, p auth.Principal, lastSent int64) (int64, error) {
	for {
		page, err := s.Read(ctx, p, lastSent, maxLimit)
		if err != nil {
			return lastSent, fmt.Errorf("reading inbox after %d: %w", lastSent, err)
		}
		if len(page) == 0 {
			return lastSent, nil
		}
		for _, e := range page {
			if err := sse.send(e); err != nil {
				return lastSent, err
			}
			lastSent = e.ID
		}
	}
}

// forwardLive drains the inbox on every wake-up signal and pings while idle.
func (s *Service) forwardLive(ctx context.Context, sse sseWriter, p auth.Principal, live <-chan []byte, lastSent int64) error {
	ping := time.NewTicker(s.heartbeat)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ping.C:
			if err := sse.ping(); err != nil {
				return err
			}
		case _, ok := <-live:
			if !ok || !coalesce(live) {
				return nil
			}
			var err error
			if lastSent, err = s.drain(ctx, sse, p, lastSent); err != nil {
				return err
			}
		}
	}
}

// coalesce discards signals already queued behind the one just received, since
// one drain covers them all. It reports false if the subscription closed.
func coalesce(live <-chan []byte) bool {
	for {
		select {
		case _, ok := <-live:
			if !ok {
				return false
			}
		default:
			return true
		}
	}
}

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
