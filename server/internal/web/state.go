package web

import (
	"context"
	"net/http"
	"slices"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/invites"
	"github.com/till-muellers/agent-message-bus/server/internal/overview"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// Overviews composes the view of one space (overview.Service).
type Overviews interface {
	Space(ctx context.Context, p auth.Principal, space string) (overview.Overview, error)
}

// Invites lists and counts the open invites of a space for its leaders and admins
// (invites.Service).
type Invites interface {
	ListPending(ctx context.Context, p auth.Principal, space string) ([]invites.Pending, error)
	CountPending(ctx context.Context, p auth.Principal, space string) (int, error)
}

// waiting is how many entries wait in the signed-in agent's mailbox scope.
type waiting struct {
	Waiting int `json:"waiting"`
}

// spaceRow is one space of the list; Invites only where the caller has leader rights.
type spaceRow struct {
	spaces.SpaceInfo
	Invites *int `json:"invites,omitempty"`
}

// listState is GET /web/api/state.
type listState struct {
	You    spaces.WhoamiResult `json:"you"`
	Spaces []spaceRow          `json:"spaces"`
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
	ctx := r.Context()
	you, err := g.spaces.Whoami(ctx, p)
	if err != nil {
		return nil, err
	}
	n, err := g.inbox.Count(ctx, p)
	if err != nil {
		return nil, err
	}
	if space := r.URL.Query().Get("space"); space != "" {
		return g.spaceState(ctx, p, space, you, waiting{Waiting: n})
	}
	return g.listState(ctx, p, you, waiting{Waiting: n})
}

func (g *Web) listState(ctx context.Context, p auth.Principal, you spaces.WhoamiResult, in waiting) (listState, error) {
	infos, err := g.spaces.List(ctx, p)
	if err != nil {
		return listState{}, err
	}
	rows := make([]spaceRow, 0, len(infos))
	for _, info := range infos {
		row := spaceRow{SpaceInfo: info}
		if leadsListed(p, info) {
			n, err := g.invites.CountPending(ctx, p, info.Name)
			if err != nil {
				return listState{}, err
			}
			row.Invites = &n
		}
		rows = append(rows, row)
	}
	return listState{You: you, Spaces: rows, Inbox: in}, nil
}

// leadsListed reports whether p has leader rights in the listed space: admins in every
// space, leaders where they lead.
func leadsListed(p auth.Principal, info spaces.SpaceInfo) bool {
	return p.Kind == auth.Admin || (p.Kind == auth.Leader && slices.Contains(info.Leaders, p.AgentName))
}

func (g *Web) spaceState(ctx context.Context, p auth.Principal, space string, you spaces.WhoamiResult, in waiting) (spaceState, error) {
	ov, err := g.overview.Space(ctx, p, space)
	if err != nil {
		return spaceState{}, err
	}
	out := spaceState{You: you, Overview: ov, Inbox: in}
	if p.Kind == auth.Admin || (ov.You.Role != nil && *ov.You.Role == spaces.RoleLeader) {
		pending, err := g.invites.ListPending(ctx, p, space)
		if err != nil {
			return spaceState{}, err
		}
		out.Invites = &pending
	}
	return out, nil
}
