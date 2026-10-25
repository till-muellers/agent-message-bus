package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/domain"
	"github.com/till-muellers/agent-message-bus/server/internal/invites"
)

type inviteArgs struct {
	Space string  `json:"space"`
	Name  string  `json:"name"`
	Task  *string `json:"task"`
}

func inviteTools(inv *invites.Service) []Tool {
	desc := fmt.Sprintf("Leaders of the space only. Returns a single-use invite code for the member agent, valid for %d minutes; "+
		"redeeming it (POST /v1/invites/accept with the code as bearer, or /bus accept <url> <code> in omp) joins the space as that member "+
		"and returns task, if given. bus_revoke_member cancels pending codes.", int(invites.TTL/time.Minute))
	return []Tool{
		define("bus_invite_member", desc, "{name, space, code, expires_at}",
			memberReq(), inviteProps(), func(ctx context.Context, p auth.Principal, a inviteArgs) (any, error) {
				i, err := inv.Create(ctx, p, a.Space, a.Name, a.Task)
				if err != nil {
					return nil, err
				}
				return map[string]string{"name": i.Name, "space": i.Space, "code": i.Code,
					"expires_at": i.ExpiresAt.UTC().Format(time.RFC3339)}, nil
			}),
	}
}

func inviteProps() map[string]any {
	props := memberProps()
	props["task"] = str(fmt.Sprintf("First prompt for the member, returned once when the code is accepted; 1..%d bytes.",
		domain.MaxBodyBytes))
	return props
}
