// Package server assembles the HTTP routes of the bus.
package server

import (
	"context"
	"net/http"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/mcpserver"
	"github.com/till-muellers/agent-message-bus/server/internal/rpc"
	"github.com/till-muellers/agent-message-bus/server/internal/tools"
)

// Routes mounts health, MCP, RPC and inbox stream endpoints.
func Routes(streamRoot context.Context, a *auth.Service, ib *inbox.Service, reg *tools.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	mux.Handle("/mcp", mcpserver.NewHandler(reg, a))
	api := a.Middleware(rpc.NewHandler(reg))
	mux.Handle("GET /v1/tools", api)
	mux.Handle("POST /v1/tools/{name}", api)
	mux.Handle("GET /v1/inbox/stream", a.Middleware(ib.StreamHandler(streamRoot)))
	return mux
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}
