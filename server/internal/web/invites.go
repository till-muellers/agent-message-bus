package web

import (
	"net/http"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
)

// cancelResult is DELETE /web/api/spaces/{space}/invites/{id}.
type cancelResult struct {
	Cancelled bool `json:"cancelled"`
}

// listInvites answers GET /web/api/spaces/{space}/invites: the open invites with their ids,
// never a code. Leader-of-space and admin sessions.
func (g *Web) listInvites(r *http.Request, p auth.Principal) (any, error) {
	return g.invites.ListPending(r.Context(), p, r.PathValue("space"))
}

// cancelInvite answers DELETE /web/api/spaces/{space}/invites/{id}; it never reveals
// whether the id existed, and it logs only when something was cancelled.
func (g *Web) cancelInvite(r *http.Request, p auth.Principal) (any, error) {
	space := r.PathValue("space")
	name, cancelled, err := g.invites.Cancel(r.Context(), p, space, r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	if cancelled {
		g.logMutation(r.Context(), p, mutationLog{"cancel-invite", space, name})
	}
	return cancelResult{Cancelled: cancelled}, nil
}
