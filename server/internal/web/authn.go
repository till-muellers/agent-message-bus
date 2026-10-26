package web

import (
	"context"
	"net/http"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpx"
)

const cookieName = "amb_session"

// signedIn is the session of a request together with its freshly re-validated principal.
type signedIn struct {
	session
	principal auth.Principal
}

type signedInKey struct{}

// authn requires a live session cookie and re-validates the session's token on every
// request: a token that stopped authenticating (rotated, revoked) ends the session.
func (g *Web) authn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := g.sessionOf(r)
		if !ok {
			httpx.WriteError(w, apperr.New(apperr.Unauthenticated, "session required"))
			return
		}
		p, err := g.auth.AuthenticateHash(r.Context(), sess.tokenHash)
		if err != nil {
			if kind, _ := apperr.KindOf(err); kind == apperr.Unauthenticated {
				g.sessions.drop(sess.id)
			}
			httpx.WriteError(w, err)
			return
		}
		ctx := context.WithValue(r.Context(), signedInKey{}, signedIn{session: sess, principal: p})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// sessionOf returns the live session named by r's cookie.
func (g *Web) sessionOf(r *http.Request) (session, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return session{}, false
	}
	return g.sessions.get(c.Value)
}

// current returns the session stored by authn.
func current(ctx context.Context) (signedIn, bool) {
	s, ok := ctx.Value(signedInKey{}).(signedIn)
	return s, ok
}
