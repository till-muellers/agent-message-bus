package web

import (
	"context"
	"net/http"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/invites"
	"github.com/till-muellers/agent-message-bus/server/internal/overview"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// Overviews composes the GUI state (overview.Service).
type Overviews interface {
	ListState(ctx context.Context, p auth.Principal) (overview.ListState, error)
	SpaceState(ctx context.Context, p auth.Principal, space string) (overview.SpaceState, error)
}

// Invites lists and cancels the open invites of a space for its leaders and admins
// (invites.Service).
type Invites interface {
	ListPending(ctx context.Context, p auth.Principal, space string) ([]invites.Pending, error)
	Cancel(ctx context.Context, p auth.Principal, space, id string) (name string, cancelled bool, err error)
}

// waiting is how many entries wait in the signed-in agent's mailbox scope.
type waiting struct {
	Waiting int `json:"waiting"`
}

// listState is GET /web/api/state; a row's invites only where the caller has leader rights.
type listState struct {
	You    spaces.WhoamiResult `json:"you"`
	Spaces []overview.SpaceRow `json:"spaces"`
	Inbox  waiting             `json:"inbox"`
}

// spaceState is GET /web/api/state?space=; Invites only for leader-rights sessions.
type spaceState struct {
	You      spaces.WhoamiResult `json:"you"`
	Overview overview.Overview   `json:"overview"`
	Invites  *[]invites.Pending  `json:"invites,omitempty"`
	Inbox    waiting             `json:"inbox"`
}

// state answers GET /web/api/state: the space list, or one space's view with ?space=.
// It carries no message bodies and never an invite code.
func (g *Web) state(r *http.Request, p auth.Principal) (any, error) {
	if space := r.URL.Query().Get("space"); space != "" {
		st, err := g.overview.SpaceState(r.Context(), p, space)
		if err != nil {
			return nil, err
		}
		return spaceState{You: st.You, Overview: st.Overview, Invites: st.Invites, Inbox: waiting{st.Waiting}}, nil
	}
	st, err := g.overview.ListState(r.Context(), p)
	if err != nil {
		return nil, err
	}
	return listState{You: st.You, Spaces: st.Spaces, Inbox: waiting{st.Waiting}}, nil
}
