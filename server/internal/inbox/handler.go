package inbox

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpx"
)

// StreamHandler serves GET /v1/inbox/stream as server-sent events: the stored
// backlog, then live entries, until the request, root or subscription ends, the
// caller's token stops authenticating or the client stops reading.
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
	p, err := auth.RequirePrincipal(r.Context())
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if !h.svc.presence.acquire(p.AgentID) {
		httpx.WriteError(w, apperr.New(apperr.Conflict, "agent %s already has an active inbox stream", p.AgentName))
		return
	}
	defer h.svc.presence.release(p.AgentID)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stop := context.AfterFunc(h.root, cancel)
	defer stop()
	logStreamEnd(ctx, h.root, p.AgentID, h.svc.stream(ctx, w, p))
}

// logStreamEnd logs why a stream ended: Info for a client that stopped reading or a
// revoked token, nothing for a disconnect, shutdown or dropped subscription, ERROR
// otherwise. A failed write cancels the request context, so a frame write that ran out
// of time counts as a client that stopped reading unless root (shutdown) cut it.
func logStreamEnd(ctx, root context.Context, agentID int64, err error) {
	switch {
	case errors.Is(err, os.ErrDeadlineExceeded) && root.Err() == nil:
		slog.InfoContext(ctx, "inbox stream dropped: client not reading", "agent_id", agentID)
	case err == nil || ctx.Err() != nil || errors.Is(err, errDropped):
	case isRevoked(err):
		slog.InfoContext(ctx, "inbox stream ended: token no longer valid", "agent_id", agentID)
	default:
		slog.ErrorContext(ctx, "inbox stream failed", "agent_id", agentID, "err", err)
	}
}
