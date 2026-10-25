package spaces

import (
	"context"
	"fmt"

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
	res := WhoamiResult{Kind: string(p.Kind), Spaces: []SpaceRole{}}
	if p.Kind == auth.Admin {
		return res, nil
	}
	res.Name = p.AgentName
	if p.Kind == auth.Member {
		err := s.db.QueryRowContext(ctx, `SELECT name FROM spaces WHERE id = ?`, p.SpaceID).Scan(&res.Space)
		if err != nil {
			return WhoamiResult{}, fmt.Errorf("looking up member space: %w", err)
		}
	}
	spaces, err := agentSpaces(ctx, s.db, p.AgentID)
	if err != nil {
		return WhoamiResult{}, err
	}
	res.Spaces = spaces
	return res, nil
}
