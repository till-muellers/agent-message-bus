package tools

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

type noArgs struct{}

type spaceArgs struct {
	Space string `json:"space"`
}

type memberArgs struct {
	Space string `json:"space"`
	Name  string `json:"name"`
}

var (
	spaceOnly   = map[string]any{"space": spaceProp}
	memberProps = map[string]any{"space": spaceProp, "name": str("Member agent name.")}
	spaceReq    = []string{"space"}
	memberReq   = []string{"space", "name"}
)

func spaceTools(s *spaces.Service) []Tool {
	return append(spaceIdentityTools(s), append(spaceLifecycleTools(s), memberTools(s)...)...)
}

func spaceIdentityTools(s *spaces.Service) []Tool {
	return []Tool{
		define("bus_whoami", "Any token. Returns the caller's kind, name and the spaces it participates in; no side effects.",
			nil, nil, func(ctx context.Context, p auth.Principal, _ noArgs) (any, error) {
				return s.Whoami(ctx, p)
			}),
		define("bus_list_spaces", "Leaders see all spaces; members see only their own space. No side effects.",
			nil, nil, func(ctx context.Context, p auth.Principal, _ noArgs) (any, error) {
				return list(s.List(ctx, p))
			}),
		define("bus_list_participants", "Participants only. Lists the space's leaders and members; no side effects.",
			spaceReq, spaceOnly, func(ctx context.Context, p auth.Principal, a spaceArgs) (any, error) {
				return list(s.Participants(ctx, p, a.Space))
			}),
	}
}

func spaceLifecycleTools(s *spaces.Service) []Tool {
	return []Tool{
		spaceOp("bus_create_space", "Leaders only. Creates the space with its all topic and joins the caller as leader.", s.Create),
		spaceOp("bus_join_space", "Leaders only. Joins an existing space as a leader.", s.Join),
		spaceOp("bus_leave_space", "Participants only; the last leader cannot leave. Removes the caller's inbox entries and subscriptions; members also lose their blackboards and tokens.", s.Leave),
		spaceOp("bus_delete_space", "Leaders only, once no other leader remains. Deletes the space and everything in it, including member tokens.", s.Delete),
	}
}

func spaceOp(name, desc string, fn func(context.Context, auth.Principal, string) error) Tool {
	return define(name, desc, spaceReq, spaceOnly, func(ctx context.Context, p auth.Principal, a spaceArgs) (any, error) {
		return done(fn(ctx, p, a.Space))
	})
}

func memberTools(s *spaces.Service) []Tool {
	return []Tool{
		define("bus_invite_member", "Leaders of the space only. Adds the member agent if needed and returns a new member token valid for this space.",
			memberReq, memberProps, func(ctx context.Context, p auth.Principal, a memberArgs) (any, error) {
				token, err := s.Invite(ctx, p, a.Space, a.Name)
				if err != nil {
					return nil, err
				}
				return map[string]string{"name": a.Name, "space": a.Space, "token": token}, nil
			}),
		define("bus_revoke_member", "Leaders of the space only. Removes the member, its tokens, inbox entries and blackboards.",
			memberReq, memberProps, func(ctx context.Context, p auth.Principal, a memberArgs) (any, error) {
				return done(s.Revoke(ctx, p, a.Space, a.Name))
			}),
	}
}
