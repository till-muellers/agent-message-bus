package tools

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

type nameArgs struct {
	Name string `json:"name"`
}

func adminTools(a *auth.Service, s *spaces.Service) []Tool {
	return []Tool{
		mintTool(BusMintLeader, "Creates the leader agent if missing and returns a new leader token for it.",
			"Leader agent", a.MintLeader),
		mintTool(BusMintAdmin, "Creates the admin agent if missing and returns a new admin token for it; "+
			"minting a name again adds a token. The bootstrap admin `admin` cannot be minted.",
			"Admin agent", a.MintAdmin),
		define(BusListAdmins, "Admins only; no side effects.",
			"Lists every admin with its bootstrap flag, token count and joined spaces.",
			"[{name, bootstrap, tokens, spaces}]",
			nil, nil, func(ctx context.Context, p auth.Principal, _ noArgs) (any, error) {
				return list(a.ListAdmins(ctx, p))
			}),
		define(BusRevokeAdmin, accessAdmins,
			"Deletes every token of the minted admin and removes it from every space it joined; spaces left empty are "+
				"deleted. `deleted` says whether anything was removed. The bootstrap admin cannot be revoked "+
				"(rotate its token with agent-message-bus rotate-token); an admin may revoke itself; minting the name again gives a new token.",
			"{deleted}",
			[]string{"name"}, map[string]any{"name": nameProp("Admin agent")},
			func(ctx context.Context, p auth.Principal, x nameArgs) (any, error) {
				deleted, err := s.RevokeAdmin(ctx, p, x.Name)
				if err != nil {
					return nil, err
				}
				return map[string]bool{"deleted": deleted}, nil
			}),
	}
}

// mintTool defines an admin tool that mints a token for agent name with mint.
func mintTool(name, desc, agent string, mint func(context.Context, auth.Principal, string) (string, error)) Tool {
	return define(name, accessAdmins, desc, "{name, token}",
		[]string{"name"}, map[string]any{"name": newNameProp(agent)},
		func(ctx context.Context, p auth.Principal, x nameArgs) (any, error) {
			token, err := mint(ctx, p, x.Name)
			if err != nil {
				return nil, err
			}
			return map[string]string{"name": x.Name, "token": token}, nil
		})
}
