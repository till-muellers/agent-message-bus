package server

import (
	"errors"
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

// drainUnreadBody sends the answer of a Connection: close request whose body the handler did
// not read to EOF, then reads the rest of the body. net/http skips its own discard for such
// requests and closes the socket at once, so a body still in flight makes the server's stack
// answer with a reset and the client loses the response (an early 401, 404 or 405).
// Keep-alive requests are drained by net/http itself.
func drainUnreadBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !r.Close || r.ContentLength == 0 {
			next.ServeHTTP(w, r)
			return
		}
		body := &eofBody{ReadCloser: r.Body}
		r.Body = body
		next.ServeHTTP(w, r)
		if !body.eof {
			drain(w, body)
		}
	})
}

// drain flushes the answer, then discards up to maxDrainBytes of body within drainTimeout.
// Its errors are dropped on purpose: a failed flush or drain only leaves the close as it was.
func drain(w http.ResponseWriter, body io.ReadCloser) {
	rc := http.NewResponseController(w)
	_ = rc.Flush()
	_ = rc.SetReadDeadline(time.Now().Add(drainTimeout))
	_, _ = io.Copy(io.Discard, http.MaxBytesReader(w, body, maxDrainBytes))
}

// eofBody records whether the body was read to its end.
type eofBody struct {
	io.ReadCloser
	eof bool
}

func (b *eofBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if errors.Is(err, io.EOF) {
		b.eof = true
	}
	return n, err
}
