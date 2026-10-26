// Package web is the backend of the web GUI: the embedded application, cookie sessions
// that are re-validated on every request, and the JSON reads under /web/api/.
// It is a transport like internal/rpc: it reads through the feature services and owns no SQL.
// It is served on its own listener (AMB_WEB_ADDR), never on the API listener.
package web

import (
	"context"
	"io/fs"
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

// Spaces describes a principal and lists the spaces it sees (spaces.Service).
type Spaces interface {
	Whoami(ctx context.Context, p auth.Principal) (spaces.WhoamiResult, error)
	List(ctx context.Context, p auth.Principal) ([]spaces.SpaceInfo, error)
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
}

// New returns the GUI backend over d.
func New(d Deps) *Web {
	return &Web{auth: d.Auth, spaces: d.Spaces, overview: d.Overview, invites: d.Invites, inbox: d.Inbox,
		tools: d.Tools, assets: assets{dist: d.Dist}, sessions: newSessions(d.Now), limit: newLimiter(d.Now)}
}

// Mount registers the GUI routes on mux. Every route is GET-only except /web/session, so
// other methods get the mux's 405. Wrap the mux with Headers.
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
	mux.Handle("GET /web/api/", noStore(g.authn(http.NotFoundHandler())))
}

// read answers one JSON read for the signed-in principal p.
type read func(r *http.Request, p auth.Principal) (any, error)

// api serves fn behind the session: its result as 200 JSON, its error in the bus envelope.
func (g *Web) api(fn read) http.Handler {
	return noStore(g.authn(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	})))
}
