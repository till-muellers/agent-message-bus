// Package rpc serves the tool registry as a JSON RPC web API.
package rpc

import (
	"errors"
	"io"
	"net/http"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpx"
	"github.com/till-muellers/agent-message-bus/server/internal/tools"
)

const maxBody = 1 << 20

type toolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// NewHandler serves GET /v1/tools and POST /v1/tools/{name}; it expects auth.Middleware to run first.
// A successful call result gains the "mail" sibling when notices has one for the caller.
func NewHandler(reg *tools.Registry, notices Noticer) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/tools", func(w http.ResponseWriter, _ *http.Request) { listTools(w, reg) })
	mux.HandleFunc("POST /v1/tools/{name}", func(w http.ResponseWriter, r *http.Request) { callTool(w, r, reg, notices) })
	return mux
}

func listTools(w http.ResponseWriter, reg *tools.Registry) {
	ts := reg.List()
	out := make([]toolInfo, 0, len(ts))
	for _, t := range ts {
		out = append(out, toolInfo{Name: t.Name, Description: t.Description, InputSchema: t.Schema})
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func callTool(w http.ResponseWriter, r *http.Request, reg *tools.Registry, notices Noticer) {
	p, ok := auth.FromContext(r.Context())
	if !ok {
		httpx.WriteError(w, apperr.New(apperr.Unauthenticated, "bearer token required"))
		return
	}
	name := r.PathValue("name")
	t, found := reg.Get(name)
	if !found {
		httpx.WriteError(w, apperr.New(apperr.NotFound, "tool %s not found", name))
		return
	}
	args, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		httpx.WriteError(w, bodyError(err))
		return
	}
	result, err := t.Call(r.Context(), p, args)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, success{Result: result, Mail: mailFor(r.Context(), notices, p)})
}

func bodyError(err error) error {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return apperr.New(apperr.Invalid, "request body exceeds 1 MiB")
	}
	return apperr.New(apperr.Invalid, "reading request body: %v", err)
}
