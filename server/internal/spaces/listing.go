package spaces

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
)

// SpaceInfo summarises a space for listing. Admins are the joined admins; Members
// counts role member only.
type SpaceInfo struct {
	Name    string   `json:"name"`
	Leaders []string `json:"leaders"`
	Admins  []string `json:"admins"`
	Members int      `json:"members"`
	Joined  bool     `json:"joined"`
}

// Listed is one listed space with the listing caller's membership in it (Role "" where
// the caller has not joined), so callers can ask Membership rules per space.
type Listed struct {
	SpaceInfo
	Caller Membership
}

// List returns the spaces visible to p: all for leaders and admins, its own for members.
func (s *Service) List(ctx context.Context, p auth.Principal) ([]SpaceInfo, error) {
	listed, err := s.Listing(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]SpaceInfo, 0, len(listed))
	for _, l := range listed {
		out = append(out, l.SpaceInfo)
	}
	return out, nil
}

// Listing is List with p's membership in each listed space.
func (s *Service) Listing(ctx context.Context, p auth.Principal) ([]Listed, error) {
	rows, err := listSpaces(ctx, s.db, p.SpaceID)
	if err != nil {
		return nil, err
	}
	out := make([]Listed, 0, len(rows))
	for _, row := range rows {
		ps, ids, err := listParticipants(ctx, s.db, row.id)
		if err != nil {
			return nil, err
		}
		caller := Membership{SpaceID: row.id, SpaceName: row.name, AgentID: p.AgentID, AgentName: p.AgentName,
			Role: roleOf(ps, ids, p.AgentID), Admin: p.Kind == auth.Admin}
		out = append(out, Listed{SpaceInfo: summarize(row.name, ps, p.AgentName), Caller: caller})
	}
	return out, nil
}

// roleOf returns the role of agentID among the participants ps (ids in the same order),
// or "" when it does not participate.
func roleOf(ps []Participant, ids []int64, agentID int64) Role {
	for i, id := range ids {
		if id == agentID {
			return ps[i].Role
		}
	}
	return ""
}

func summarize(name string, ps []Participant, caller string) SpaceInfo {
	info := SpaceInfo{Name: name, Leaders: []string{}, Admins: []string{}}
	for _, pt := range ps {
		switch pt.Role {
		case RoleAdmin:
			info.Admins = append(info.Admins, pt.Name)
		case RoleLeader:
			info.Leaders = append(info.Leaders, pt.Name)
		default:
			info.Members++
		}
		if pt.Name == caller {
			info.Joined = true
		}
	}
	return info
}
