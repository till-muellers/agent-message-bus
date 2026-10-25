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

// stream subscribes before reading the backlog so nothing published meanwhile
// is lost; lastSent then drops live copies of entries the backlog delivered.
func (s *Service) stream(ctx context.Context, w http.ResponseWriter, p auth.Principal) error {
	live, unsubscribe := s.pub.Subscribe(topic(p.AgentID))
	defer unsubscribe()
	sse, err := openSSE(w)
	if err != nil {
		return err
	}
	lastSent, err := s.sendBacklog(ctx, sse, p)
	if err != nil {
		return err
	}
	return s.forwardLive(ctx, sse, p, live, lastSent)
}

// sendBacklog sends stored entries page by page and returns the last id sent.
func (s *Service) sendBacklog(ctx context.Context, sse sseWriter, p auth.Principal) (int64, error) {
	var lastSent int64
	for {
		page, err := s.Read(ctx, p, lastSent, maxLimit)
		if err != nil {
			return lastSent, fmt.Errorf("reading backlog: %w", err)
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
		case payload, ok := <-live:
			if !ok {
				return nil
			}
			var err error
			if lastSent, err = forward(sse, p, payload, lastSent); err != nil {
				return err
			}
		}
	}
}

// forward sends one live payload unless it was already sent or lies outside
// p's space, and returns the new lastSent.
func forward(sse sseWriter, p auth.Principal, payload []byte, lastSent int64) (int64, error) {
	var env envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return lastSent, fmt.Errorf("decoding live inbox entry: %w", err)
	}
	e := env.Entry
	e.AgentID, e.SpaceID = p.AgentID, env.SpaceID
	if e.ID <= lastSent || !visible(p, e.SpaceID) {
		return lastSent, nil
	}
	if err := sse.send(e); err != nil {
		return lastSent, err
	}
	return e.ID, nil
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
