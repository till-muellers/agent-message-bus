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
// subscription ends the wait with one more read. Wait never acknowledges and never takes
// the stream slot.
func (s *Service) Wait(ctx context.Context, p auth.Principal, q WaitQuery) ([]Entry, error) {
	live, unsubscribe := s.pub.Subscribe(topic(p.AgentID))
	defer unsubscribe()
	es, err := s.matching(ctx, p, q)
	if err != nil || len(es) > 0 {
		return es, err
	}
	timer := time.NewTimer(q.Timeout)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return []Entry{}, nil
		case <-s.root.Done():
			return []Entry{}, nil
		case <-timer.C:
			return []Entry{}, nil
		case _, ok := <-live:
			es, err := s.matching(ctx, p, q)
			if err != nil || len(es) > 0 || !ok {
				return es, err
			}
		}
	}
}

// matching reads p's entries selected by q, at most waitLimit. A read cut short because
// the caller went away ends the wait with [] like any other disconnect.
func (s *Service) matching(ctx context.Context, p auth.Principal, q WaitQuery) ([]Entry, error) {
	es, err := selectMatching(ctx, s.db, p.AgentID, spaceFilter(p), q)
	if err != nil && ctx.Err() != nil {
		return []Entry{}, nil
	}
	return es, err
}
