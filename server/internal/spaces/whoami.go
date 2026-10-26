package spaces

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
)

// SpaceRole is one space the caller participates in.
type SpaceRole struct {
	Name string `json:"name"`
	Role Role   `json:"role"`
}

// WhoamiResult describes the authenticated caller.
type WhoamiResult struct {
	Kind   string      `json:"kind"`
	Name   string      `json:"name,omitempty"`
	Space  string      `json:"space,omitempty"`
	Spaces []SpaceRole `json:"spaces"`
}

// Whoami returns p's identity and the spaces it participates in.
func (s *Service) Whoami(ctx context.Context, p auth.Principal) (WhoamiResult, error) {
	res := WhoamiResult{Kind: string(p.Kind), Name: p.AgentName, Spaces: []SpaceRole{}}
	if p.Kind == auth.Member {
		name, err := spaceName(ctx, s.db, p.SpaceID)
		if err != nil {
			return WhoamiResult{}, err
		}
		res.Space = name
	}
	spaces, err := agentSpaces(ctx, s.db, p.AgentID)
	if err != nil {
		return WhoamiResult{}, err
	}
	res.Spaces = spaces
	return res, nil
}
