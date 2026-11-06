package inbox

import (
	"context"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

const (
	defaultWait = 30 * time.Second
	maxWait     = 300 * time.Second
	waitLimit   = 100
)

// WaitQuery selects the entries a Wait returns: id > After, from sender From and of
// thread ThreadID when set. Timeout bounds the wait.
type WaitQuery struct {
	After    int64
	From     string
	ThreadID int64
	Timeout  time.Duration
}

// WaitTimeout turns the optional timeoutSeconds argument into a duration: 30 s when
// absent, otherwise 1..300 s.
func WaitTimeout(seconds *int) (time.Duration, error) {
	if seconds == nil {
		return defaultWait, nil
	}
	d := time.Duration(*seconds) * time.Second
	if d < time.Second || d > maxWait {
		return 0, apperr.New(apperr.Invalid, "timeoutSeconds must be 1..300")
	}
	return d, nil
}

// Wait returns p's matching unacknowledged entries at once, or blocks until one arrives,
// the timeout passes, ctx ends or the service's root ends (each → []). A dropped live
// subscription ends the wait with one more read. After every read and on every heartbeat
// it re-validates p's token: once it stopped authenticating, Wait fails with
// Unauthenticated.
// Wait never acknowledges and never takes the stream slot, but counts p as connected while
// it runs; it marks the edges it returns noticed (MarkNoticed).
func (s *Service) Wait(ctx context.Context, p auth.Principal, q WaitQuery) ([]Entry, error) {
	s.presence.enterWait(p.AgentID)
	defer s.presence.leaveWait(p.AgentID)
	live, unsubscribe := s.pub.Subscribe(topic(p.AgentID))
	defer unsubscribe()
	es, err := s.matching(ctx, p, q)
	if err != nil || len(es) > 0 {
		return es, err
	}
	return s.await(ctx, p, q, live)
}

// await blocks until a wake-up signal brings a matching entry, the timeout, ctx or root
// ends, or p's token stops authenticating (checked every heartbeat).
func (s *Service) await(ctx context.Context, p auth.Principal, q WaitQuery, live <-chan []byte) ([]Entry, error) {
	timer := time.NewTimer(q.Timeout)
	defer timer.Stop()
	check := time.NewTicker(s.heartbeat)
	defer check.Stop()
	for {
		select {
		case <-ctx.Done():
			return []Entry{}, nil
		case <-s.root.Done():
			return []Entry{}, nil
		case <-timer.C:
			return []Entry{}, nil
		case <-check.C:
			if err := s.stillValid(ctx, p); err != nil {
				return disconnected(ctx, nil, err)
			}
		case _, ok := <-live:
			es, err := s.matching(ctx, p, q)
			if err != nil || len(es) > 0 || !ok {
				return es, err
			}
		}
	}
}

// matching reads p's entries selected by q, at most waitLimit, re-validates p's token
// (after the read, so it sees a revocation committed before any entry read) and marks the
// edges among them noticed.
func (s *Service) matching(ctx context.Context, p auth.Principal, q WaitQuery) ([]Entry, error) {
	es, err := selectMatching(ctx, s.db, p.AgentID, spaceFilter(p), q)
	if err == nil {
		err = s.stillValid(ctx, p)
	}
	if err != nil {
		return disconnected(ctx, nil, err)
	}
	return disconnected(ctx, es, s.MarkNoticed(ctx, p, es))
}

// disconnected ends a wait whose step failed because the caller went away with [] like
// any other disconnect; other results pass through.
func disconnected(ctx context.Context, es []Entry, err error) ([]Entry, error) {
	if err != nil && ctx.Err() != nil {
		return []Entry{}, nil
	}
	return es, err
}
