// Package server assembles the HTTP routes of the bus.
package server

import (
	"context"
	"net/http"
	"strconv"

	"github.com/till-muellers/agent-message-bus/server/internal/admin"
	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/guide"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/invites"
	"github.com/till-muellers/agent-message-bus/server/internal/mcpserver"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/instance"
	"github.com/till-muellers/agent-message-bus/server/internal/rpc"
	"github.com/till-muellers/agent-message-bus/server/internal/tools"
	"github.com/till-muellers/agent-message-bus/server/internal/web"
)

// Routes mounts health, MCP, RPC, invite acceptance, inbox stream, guide and localhost admin
// endpoints: the API listener (AMB_ADDR). It has no /web route; the GUI is WebRoutes.
// /healthz names instanceID; shutdown is invoked by POST /v1/admin/shutdown. Invite acceptance skips the auth
// middleware because the invite code itself is the bearer.
func Routes(streamRoot context.Context, a *auth.Service, ib *inbox.Service, inv *invites.Service,
	reg *tools.Registry, instanceID string, shutdown func()) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz(instanceID))
	mux.Handle("/mcp", mcpserver.NewHandler(reg, a, ib))
	mux.Handle("GET /v1/tools", a.Middleware(rpc.ListTools(reg)))
	mux.Handle("POST /v1/tools/{name}", a.Middleware(rpc.CallTool(reg, ib)))
	mux.Handle("POST /v1/invites/accept", inv.AcceptHandler())
	mux.Handle("GET /v1/inbox/stream", a.Middleware(ib.StreamHandler(streamRoot)))
	mux.Handle("GET /v1/guide", a.Middleware(http.HandlerFunc(serveGuide)))
	admin.Mount(mux, a.Middleware, a, shutdown)
	return mux
}

// WebRoutes mounts the web GUI on its own mux for the GUI listener (AMB_WEB_ADDR); it serves
// only /web/*, everything else is 404.
func WebRoutes(gui *web.Web) http.Handler {
	mux := http.NewServeMux()
	gui.Mount(mux)
	return web.Headers(mux)
}

func healthz(instanceID string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Agent-Bus-Protocol", strconv.Itoa(Protocol))
		w.Header().Set(instance.IDHeader, instanceID)
		_, _ = w.Write([]byte("ok"))
	}
}

func serveGuide(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write([]byte(guide.Text))
}
