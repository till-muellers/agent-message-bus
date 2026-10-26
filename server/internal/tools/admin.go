package tools

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
)

type nameArgs struct {
	Name string `json:"name"`
}

func adminTools(a *auth.Service) []Tool {
	return []Tool{
		mintTool("bus_mint_leader", "Creates the leader agent if missing and returns a new leader token for it.",
			"Leader agent", a.MintLeader),
		mintTool("bus_mint_admin", "Creates the admin agent if missing and returns a new admin token for it; "+
			"minting the same name again issues an additional token. "+
			"Pitfall: admin is the bootstrap admin, whose token is the instance admin token, so it cannot be minted.",
			"Admin agent", a.MintAdmin),
		define("bus_list_admins", "Admins only; no side effects.",
			"Lists every admin with its bootstrap flag, its number of tokens and the spaces it joined.",
			"[{name, bootstrap, tokens, spaces}]",
			nil, nil, func(ctx context.Context, p auth.Principal, _ noArgs) (any, error) {
				return list(a.ListAdmins(ctx, p))
			}),
	}
}

// mintTool defines an admin tool that mints a token for agent name with mint.
func mintTool(name, desc, agent string, mint func(context.Context, auth.Principal, string) (string, error)) Tool {
	return define(name, accessAdmins, desc, "{name, token}",
		[]string{"name"}, map[string]any{"name": str(nameDesc(agent))},
		func(ctx context.Context, p auth.Principal, x nameArgs) (any, error) {
			token, err := mint(ctx, p, x.Name)
			if err != nil {
				return nil, err
			}
			return map[string]string{"name": x.Name, "token": token}, nil
		})
}
