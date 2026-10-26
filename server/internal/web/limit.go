package web

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpx"
)

const (
	maxFailures   = 5
	failureWindow = time.Minute
)

// limiter counts failed sign-ins per remote IP over a sliding minute; after maxFailures
// every sign-in from that IP answers 429 until the oldest failure is a minute old [AD-9].
type limiter struct {
	mu        sync.Mutex
	now       func() time.Time
	failures  map[string][]time.Time // oldest first, at most maxFailures
	lastSweep time.Time
}

func newLimiter(now func() time.Time) *limiter {
	return &limiter{now: now, failures: map[string][]time.Time{}}
}

// blocked reports whether ip has used up its failed sign-ins.
func (l *limiter) blocked(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(ip, l.now())) >= maxFailures
}

// fail records a failed sign-in from ip.
func (l *limiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)
	ts := l.recent(ip, now)
	if len(ts) >= maxFailures {
		ts = ts[1:]
	}
	l.failures[ip] = append(ts, now)
}

// recent drops ip's failures older than failureWindow and returns the rest. Call with mu held.
func (l *limiter) recent(ip string, now time.Time) []time.Time {
	ts := l.failures[ip]
	i := 0
	for i < len(ts) && now.Sub(ts[i]) >= failureWindow {
		i++
	}
	ts = ts[i:]
	if len(ts) == 0 {
		delete(l.failures, ip)
		return nil
	}
	l.failures[ip] = ts
	return ts
}

// sweep forgets IPs without recent failures, at most once per window. Call with mu held.
func (l *limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < failureWindow {
		return
	}
	for ip := range l.failures {
		l.recent(ip, now)
	}
	l.lastSweep = now
}

// remoteIP is the host part of r.RemoteAddr. Behind a reverse proxy it is the proxy's address.
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimited is the 429 body; written here because apperr has no rate-limit kind [AD-9].
type rateLimited struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeRateLimited(w http.ResponseWriter) {
	var body rateLimited
	body.Error.Code = "rate_limited"
	body.Error.Message = "too many failed sign-ins; try again later"
	httpx.WriteJSON(w, http.StatusTooManyRequests, body)
}
