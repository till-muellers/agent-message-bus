// Package web is the backend of the web GUI: the embedded application, cookie sessions
// that are re-validated on every request, the JSON reads under /web/api/, the mutations and
// the admin area, whose writes need the session's CSRF token. It is a transport like
// internal/rpc: it reads through the feature services, writes through the tool registry
// (and invites.Service.Cancel, which has no tool) and owns no SQL.
// It is served on its own listener (AMB_WEB_ADDR), never on the API listener.
package web

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpx"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// Authenticator resolves a stored token hash to its principal (auth.Service).
type Authenticator interface {
	AuthenticateHash(ctx context.Context, hash string) (auth.Principal, error)
}

// Spaces describes a principal (spaces.Service).
type Spaces interface {
	Whoami(ctx context.Context, p auth.Principal) (spaces.WhoamiResult, error)
}

// Deps are the services and inputs of the web GUI backend.
type Deps struct {
	Auth     Authenticator
	Spaces   Spaces
	Overview Overviews
	Invites  Invites
	Inbox    Inbox
	Tools    Tools
	Dist     fs.FS // the GUI bundle: index.html and assets/*; see Dist
	Now      func() time.Time
}

// Web serves the GUI routes.
type Web struct {
	auth     Authenticator
	spaces   Spaces
	overview Overviews
	invites  Invites
	inbox    Inbox
	tools    Tools
	assets   assets
	sessions *sessions
	limit    *limiter
	log      *slog.Logger // one "web mutation" line per GUI mutation
}

// New returns the GUI backend over d; mutation lines go to slog.Default().
func New(d Deps) *Web {
	return &Web{auth: d.Auth, spaces: d.Spaces, overview: d.Overview, invites: d.Invites, inbox: d.Inbox,
		tools: d.Tools, assets: assets{dist: d.Dist}, sessions: newSessions(d.Now), limit: newLimiter(d.Now),
		log: slog.Default()}
}

// Mount registers the GUI routes on mux. Reads are GET-only; the mutating routes are the
// session routes, the space mutations and the admin area; all but the session routes need
// X-CSRF-Token. Other methods get the mux's 405, also on an unknown /web/api path (GET and
// HEAD there are 404 not_found). Wrap the mux with Headers.
func (g *Web) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /web/", g.assets.index)
	mux.HandleFunc("GET /web/assets/", g.assets.file)
	mux.Handle("POST /web/session", noStore(http.HandlerFunc(g.signIn)))
	mux.Handle("GET /web/session", noStore(g.authn(http.HandlerFunc(g.restore))))
	mux.Handle("DELETE /web/session", noStore(http.HandlerFunc(g.signOut)))
	mux.Handle("GET /web/api/state", g.api(g.state))
	mux.Handle("GET /web/api/spaces/{space}/blackboards/{board}/threads", g.api(g.threads))
	mux.Handle("GET /web/api/spaces/{space}/threads/{id}", g.api(g.thread))
	mux.Handle("GET /web/api/inbox", g.api(g.readInbox))
	mux.Handle("POST /web/api/spaces/{space}/invites", g.mutation(g.invite))
	mux.Handle("GET /web/api/spaces/{space}/invites", g.api(g.listInvites))
	mux.Handle("DELETE /web/api/spaces/{space}/invites/{id}", g.mutation(g.cancelInvite))
	mux.Handle("POST /web/api/spaces/{space}/revoke", g.mutation(g.revoke))
	mux.Handle("POST /web/api/spaces/{space}/threads/{id}/posts", g.mutation(g.post))
	mux.Handle("POST /web/api/spaces/{space}/topics/{topic}/messages", g.mutation(g.send))
	mux.Handle("DELETE /web/api/spaces/{space}", g.mutation(g.deleteSpace))
	mux.Handle("GET /web/api/admins", g.api(g.listAdmins))
	mux.Handle("POST /web/api/admins", g.mutation(g.mintAdmin))
	mux.Handle("POST /web/api/admins/{name}/revoke", g.mutation(g.revokeAdmin))
	mux.Handle("GET /web/api/", g.api(routeNotFound))
}

// routeNotFound answers an unknown GET /web/api/<path> with the §0 envelope. Other methods on
// an unknown path still get the mux's 405: a method-less catch-all conflicts with GET /web/,
// and per-method ones would turn the 405 of known GET-only routes into 404.
func routeNotFound(*http.Request, auth.Principal) (any, error) {
	return nil, apperr.New(apperr.NotFound, "route not found")
}

// endpoint answers one JSON request for the signed-in principal p.
type endpoint func(r *http.Request, p auth.Principal) (any, error)

// api serves the read fn behind the session.
func (g *Web) api(fn endpoint) http.Handler {
	return noStore(g.authn(serve(fn)))
}

// mutation serves fn behind the session and its CSRF token.
func (g *Web) mutation(fn endpoint) http.Handler {
	return noStore(g.authn(csrf(serve(fn))))
}

// serve writes fn's result as 200 JSON and its error in the bus envelope (behind authn).
func serve(fn endpoint) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur, ok := current(r.Context())
		if !ok {
			httpx.WriteError(w, apperr.New(apperr.Unauthenticated, "session required"))
			return
		}
		out, err := fn(r, cur.principal)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	})
}
