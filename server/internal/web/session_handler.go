package web

import (
	"context"
	"net/http"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpx"
)

// sessionBody answers sign-in and restore; csrf stays the same for the session's life.
type sessionBody struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Space string `json:"space,omitempty"`
	CSRF  string `json:"csrf"`
}

// signIn exchanges "Authorization: Bearer <token>" for a session cookie (POST /web/session).
// Failed sign-ins count toward the per-IP limit; the token is never echoed.
func (g *Web) signIn(w http.ResponseWriter, r *http.Request) {
	ip := remoteIP(r)
	if g.limit.blocked(ip) {
		writeRateLimited(w)
		return
	}
	p, hash, err := g.authenticate(r)
	if err != nil {
		if kind, _ := apperr.KindOf(err); kind == apperr.Unauthenticated {
			g.limit.fail(ip)
		}
		httpx.WriteError(w, err)
		return
	}
	body, err := g.describe(r.Context(), p)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	sess, err := g.sessions.create(hash)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	http.SetCookie(w, sessionCookie(r, sess.id, int(sessionTTL/time.Second)))
	body.CSRF = sess.csrf
	httpx.WriteJSON(w, http.StatusOK, body)
}

// authenticate resolves r's bearer token and returns its principal and hash.
func (g *Web) authenticate(r *http.Request) (auth.Principal, string, error) {
	token, err := auth.BearerToken(r)
	if err != nil {
		return auth.Principal{}, "", err
	}
	h := auth.Hash(token)
	p, err := g.auth.AuthenticateHash(r.Context(), h)
	return p, h, err
}

// restore answers GET /web/session (behind authn) with the sign-in body of the session.
func (g *Web) restore(w http.ResponseWriter, r *http.Request) {
	cur, ok := current(r.Context())
	if !ok {
		httpx.WriteError(w, apperr.New(apperr.Unauthenticated, "session required"))
		return
	}
	body, err := g.describe(r.Context(), cur.principal)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	body.CSRF = cur.csrf
	httpx.WriteJSON(w, http.StatusOK, body)
}

// signOut drops the cookie's session, if any, and clears the cookie (DELETE /web/session).
func (g *Web) signOut(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		g.sessions.drop(c.Value)
	}
	http.SetCookie(w, sessionCookie(r, "", -1))
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// describe returns p's kind, name and (members) space.
func (g *Web) describe(ctx context.Context, p auth.Principal) (sessionBody, error) {
	who, err := g.who.Whoami(ctx, p)
	if err != nil {
		return sessionBody{}, err
	}
	return sessionBody{Kind: who.Kind, Name: who.Name, Space: who.Space}, nil
}

// sessionCookie is the amb_session cookie: scoped to /web so it never reaches /v1/*,
// Secure when the request arrived over TLS. maxAge < 0 clears it.
func sessionCookie(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{Name: cookieName, Value: value, Path: "/web", MaxAge: maxAge,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil}
}
