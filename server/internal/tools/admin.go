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
		define("bus_mint_leader",
			"Admin token only. Creates the leader agent if missing and returns a new leader token for it.",
			[]string{"name"}, map[string]any{"name": str(nameDesc("Leader agent"))},
			func(ctx context.Context, p auth.Principal, x nameArgs) (any, error) {
				token, err := a.MintLeader(ctx, p, x.Name)
				if err != nil {
					return nil, err
				}
				return map[string]string{"name": x.Name, "token": token}, nil
			}),
	}
}
