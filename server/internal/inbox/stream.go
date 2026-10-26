package inbox

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
)

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

// drain sends p's stored entries after lastSent page by page, marks the edges of each sent
// page noticed (the stream told the agent) and returns the last id sent.
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
		if err := s.markSeen(ctx, p.AgentID, page); err != nil {
			return lastSent, err
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
