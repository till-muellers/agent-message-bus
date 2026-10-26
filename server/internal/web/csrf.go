package web

import (
	"crypto/subtle"
	"net/http"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpx"
)

// csrfHeader carries the session's CSRF token (from sign-in or restore) on every mutating
// /web/api request.
const csrfHeader = "X-CSRF-Token"

// csrf runs behind authn: it lets the request through only when X-CSRF-Token equals the
// session's token, else 403 forbidden "invalid CSRF token" before any side effect.
// DELETE /web/session does not need it: signing out is harmless.
func csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur, ok := current(r.Context())
		if !ok || !sameToken(r.Header.Get(csrfHeader), cur.csrf) {
			httpx.WriteError(w, apperr.New(apperr.Forbidden, "invalid CSRF token"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sameToken compares in constant time; an empty token never matches.
func sameToken(got, want string) bool {
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
