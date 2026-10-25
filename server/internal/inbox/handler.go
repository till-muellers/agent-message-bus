package inbox

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpx"
)

// StreamHandler serves GET /v1/inbox/stream as server-sent events: the stored
// backlog, then live entries, until the request, root or subscription ends.
// Each agent has one stream slot: a second concurrent stream gets 409 Conflict.
type StreamHandler struct {
	svc  *Service
	root context.Context
}

// StreamHandler builds the inbox stream handler; root cancels every stream.
func (s *Service) StreamHandler(root context.Context) http.Handler {
	return StreamHandler{svc: s, root: root}
}

func (h StreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p, err := streamPrincipal(r.Context())
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if !h.svc.slots.acquire(p.AgentID) {
		httpx.WriteError(w, apperr.New(apperr.Conflict, "agent %s already has an active inbox stream", p.AgentName))
		return
	}
	defer h.svc.slots.release(p.AgentID)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stop := context.AfterFunc(h.root, cancel)
	defer stop()
	if err := h.svc.stream(ctx, w, p); err != nil && ctx.Err() == nil {
		slog.ErrorContext(ctx, "inbox stream failed", "agent_id", p.AgentID, "err", err)
	}
}

func streamPrincipal(ctx context.Context) (auth.Principal, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return auth.Principal{}, err
	}
	return p, requireAgent(p)
}
