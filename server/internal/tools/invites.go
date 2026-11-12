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
	desc := fmt.Sprintf("Returns an invite code for member agent `name`, with an optional task delivered to it as a direct message from you "+
		"when the code is accepted. The code is single-use and expires after %d minutes; the user redeems it with "+
		"`/bus accept <url> <code>` or `omp --bus-url <url> --bus-accept <code>`, other clients with POST /v1/invites/accept. "+
		"Inviting a name again gives a new code; accepting it ends the old token and clears that member's inbox and "+
		"subscriptions in the space. bus_revoke_member cancels pending codes.",
		int(invites.TTL/time.Minute))
	return []Tool{
		define(BusInviteMember, accessSpaceLeaders, desc, "{name, space, code, expires_at}",
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
	props := map[string]any{"space": spaceProp(), "name": newNameProp("Member agent")}
	props["task"] = str(fmt.Sprintf("The member's first task: goal, done criteria and where to report; 1..%d bytes. Arrives as a direct message from you.",
		domain.MaxBodyBytes))
	return props
}
