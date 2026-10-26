// Package web is the backend of the web GUI: the embedded application, cookie sessions
// that are re-validated on every request, and (later) the JSON reads under /web/api/.
// It is a transport like internal/rpc: it reads through the feature services and owns no SQL.
// It is served on its own listener (AMB_WEB_ADDR), never on the API listener.
package web

import (
	"context"
	"io/fs"
	"net/http"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// Authenticator resolves a stored token hash to its principal (auth.Service).
type Authenticator interface {
	AuthenticateHash(ctx context.Context, hash string) (auth.Principal, error)
}

// Identifier describes a principal: name, kind and a member's space (spaces.Service).
type Identifier interface {
	Whoami(ctx context.Context, p auth.Principal) (spaces.WhoamiResult, error)
}

// Deps are the services and inputs of the web GUI backend.
type Deps struct {
	Auth Authenticator
	Who  Identifier
	Dist fs.FS // the GUI bundle: index.html and assets/*; see Dist
	Now  func() time.Time
}

// Web serves the GUI routes.
type Web struct {
	auth     Authenticator
	who      Identifier
	assets   assets
	sessions *sessions
	limit    *limiter
}

// New returns the GUI backend over d.
func New(d Deps) *Web {
	return &Web{auth: d.Auth, who: d.Who, assets: assets{dist: d.Dist},
		sessions: newSessions(d.Now), limit: newLimiter(d.Now)}
}

// Mount registers the GUI routes on mux. Every route is GET-only except /web/session, so
// other methods get the mux's 405. Wrap the mux with Headers.
func (g *Web) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /web/", g.assets.index)
	mux.HandleFunc("GET /web/assets/", g.assets.file)
	mux.Handle("POST /web/session", noStore(http.HandlerFunc(g.signIn)))
	mux.Handle("GET /web/session", noStore(g.authn(http.HandlerFunc(g.restore))))
	mux.Handle("DELETE /web/session", noStore(http.HandlerFunc(g.signOut)))
	mux.Handle("GET /web/api/", noStore(g.authn(http.NotFoundHandler())))
}
