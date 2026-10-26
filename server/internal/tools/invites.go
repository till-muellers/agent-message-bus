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
	desc := fmt.Sprintf("Returns an invite code that lets member agent <name> join the space, with an optional task "+
		"delivered as a direct message from you when the code is accepted. Pitfalls: the code is single-use and expires after %d minutes; the user redeems it with "+
		"/bus accept <url> <code> or omp --bus-url <url> --bus-accept <code>, other clients with POST /v1/invites/accept; "+
		"inviting the same name again gives a new code and an additional token; bus_revoke_member cancels pending codes.",
		int(invites.TTL/time.Minute))
	return []Tool{
		define("bus_invite_member", accessSpaceLeaders, desc, "{name, space, code, expires_at}",
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
	props["task"] = str(fmt.Sprintf("First prompt for the member, delivered as a direct message from you when the code is accepted; 1..%d bytes.",
		domain.MaxBodyBytes))
	return props
}
