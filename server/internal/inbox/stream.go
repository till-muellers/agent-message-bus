package inbox

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
)

// errDropped ends a stream whose live subscription the broker closed.
var errDropped = errors.New("inbox subscription dropped")

// stream subscribes before draining the backlog so nothing published meanwhile
// is missed. Live messages are only wake-up signals: each one drains the inbox
// after lastSent, so entries go out once and in id order even when producers
// publish out of order. Once ctx ends, a write blocked on the client fails within
// expireGrace.
func (s *Service) stream(ctx context.Context, w http.ResponseWriter, p auth.Principal) error {
	live, unsubscribe := s.pub.Subscribe(topic(p.AgentID))
	defer unsubscribe()
	sse, err := openSSE(w, s.frameTimeout, ctx.Done())
	if err != nil {
		return err
	}
	finish := sse.expireWhenDone(ctx)
	err = s.deliver(ctx, sse, p, live)
	if ferr := finish(); err == nil {
		err = ferr
	}
	return err
}

// deliver sends the backlog, then live entries, until the stream ends.
func (s *Service) deliver(ctx context.Context, sse sseWriter, p auth.Principal, live <-chan []byte) error {
	lastSent, err := s.drain(ctx, sse, p, live, 0)
	if err != nil {
		return err
	}
	return s.forwardLive(ctx, sse, p, live, lastSent)
}

// drain sends p's stored entries after lastSent page by page and returns the last id
// sent. It re-validates p's token after each read and before sending what it read, so a
// revocation that committed before an entry was stored is seen before that entry goes
// out, and a woken stream of a dead token ends even when nothing in its scope arrived.
// The read that finds nothing more after a page is not checked, since that page's check
// covered it: one token query per wake-up and per further page. After each page it
// discards the queued wake-up signals, since the next Read covers them, so a long drain
// under a sender burst does not overflow the subscription.
func (s *Service) drain(ctx context.Context, sse sseWriter, p auth.Principal, live <-chan []byte, lastSent int64) (int64, error) {
	for sent := false; ; sent = true {
		page, err := s.Read(ctx, p, lastSent, MaxLimit)
		if err != nil {
			return lastSent, fmt.Errorf("reading inbox after %d: %w", lastSent, err)
		}
		if sent && len(page) == 0 {
			return lastSent, nil
		}
		if err := s.stillValid(ctx, p); err != nil || len(page) == 0 {
			return lastSent, err
		}
		if lastSent, err = s.sendPage(ctx, sse, p, page, lastSent); err != nil {
			return lastSent, err
		}
		if !coalesce(live) {
			return lastSent, errDropped
		}
	}
}

// sendPage sends page, marks its edges noticed (the stream told the agent) and returns
// the last id sent.
func (s *Service) sendPage(ctx context.Context, sse sseWriter, p auth.Principal, page []Entry, lastSent int64) (int64, error) {
	for _, e := range page {
		if err := sse.send(e); err != nil {
			return lastSent, err
		}
		lastSent = e.ID
	}
	return lastSent, s.MarkNoticed(ctx, p, page)
}

// forwardLive drains the inbox on every wake-up signal and pings while idle; each ping
// re-validates p's token first, so an idle stream of a revoked token ends within one
// heartbeat.
func (s *Service) forwardLive(ctx context.Context, sse sseWriter, p auth.Principal, live <-chan []byte, lastSent int64) error {
	ping := time.NewTicker(s.heartbeat)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ping.C:
			if err := s.stillValid(ctx, p); err != nil {
				return err
			}
			if err := sse.ping(); err != nil {
				return err
			}
		case _, ok := <-live:
			if !ok || !coalesce(live) {
				return nil
			}
			var err error
			if lastSent, err = s.drain(ctx, sse, p, live, lastSent); err != nil {
				return err
			}
		}
	}
}

// coalesce discards the wake-up signals already queued, since the next drain or page
// read covers them all. It reports false if the subscription closed.
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
