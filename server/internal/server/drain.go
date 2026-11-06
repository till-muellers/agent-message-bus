package server

import (
	"io"
	"net/http"
	"time"
)

const (
	// maxDrainBytes bounds how much of an unread body is read; past it net/http closes the
	// connection with its own lingering close.
	maxDrainBytes = 1 << 20
	// drainTimeout bounds the wait for a body the client declared but does not send.
	drainTimeout = 5 * time.Second
)

// drainUnreadBody reads what the handler left of a Connection: close request's body.
// net/http skips its own discard for such requests and closes the socket at once, so a body
// still in flight makes the server's stack answer with a reset and the client loses the
// response already sent (an early 401, 404 or 405). Keep-alive requests are drained by
// net/http itself.
func drainUnreadBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if r.Close && r.ContentLength != 0 {
			drain(w, r)
		}
	})
}

// drain discards up to maxDrainBytes of r.Body within drainTimeout. Its errors are dropped on
// purpose: the response is already written, and a failed drain only leaves the close as it was.
func drain(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(drainTimeout))
	_, _ = io.Copy(io.Discard, http.MaxBytesReader(w, r.Body, maxDrainBytes))
}
