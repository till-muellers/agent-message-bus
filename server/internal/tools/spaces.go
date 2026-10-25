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

func spaceTools(s *spaces.Service) []Tool {
	return append(spaceIdentityTools(s), append(spaceLifecycleTools(s), memberTools(s)...)...)
}

func spaceIdentityTools(s *spaces.Service) []Tool {
	return []Tool{
		define("bus_whoami", "Any token. Returns the caller's kind, name and the spaces it participates in; no side effects.",
			"{kind, name?, space?, spaces: [{name, role}]}",
			nil, nil, func(ctx context.Context, p auth.Principal, _ noArgs) (any, error) {
				return s.Whoami(ctx, p)
			}),
		define("bus_list_spaces", "Leaders see all spaces; members see only their own space. No side effects.",
			"[{name, leaders, members, joined}]",
			nil, nil, func(ctx context.Context, p auth.Principal, _ noArgs) (any, error) {
				return list(s.List(ctx, p))
			}),
		define("bus_list_participants", "Participants only. Lists the space's leaders and members; no side effects.",
			"[{name, role}]",
			spaceReq(), spaceOnly(), func(ctx context.Context, p auth.Principal, a spaceArgs) (any, error) {
				return list(s.Participants(ctx, p, a.Space))
			}),
	}
}

func spaceLifecycleTools(s *spaces.Service) []Tool {
	return []Tool{
		spaceOp("bus_create_space", "Leaders only. Creates the space with its all topic and joins the caller as leader.", s, (*spaces.Service).Create),
		spaceOp("bus_join_space", "Leaders only. Joins an existing space as a leader.", s, (*spaces.Service).Join),
		spaceOp("bus_leave_space", "Participants only; the last leader cannot leave. Removes the caller's inbox entries and subscriptions; members also lose their blackboards and tokens.", s, (*spaces.Service).Leave),
		spaceOp("bus_delete_space", "Leaders only, once no other leader remains. Deletes the space and everything in it, including member tokens.", s, (*spaces.Service).Delete),
	}
}

// spaceOp binds fn to s only when the tool is called.
func spaceOp(name, desc string, s *spaces.Service,
	fn func(*spaces.Service, context.Context, auth.Principal, string) error) Tool {
	return define(name, desc, okResult, spaceReq(), spaceOnly(), func(ctx context.Context, p auth.Principal, a spaceArgs) (any, error) {
		return done(fn(s, ctx, p, a.Space))
	})
}

func memberTools(s *spaces.Service) []Tool {
	return []Tool{
		define("bus_revoke_member", "Leaders of the space only; leaders cannot be revoked. Idempotent: removes the member, if present, with its tokens, inbox entries and blackboards, and cancels its pending invite codes for the space. deleted reports whether anything was removed.",
			"{deleted}",
			memberReq(), memberProps(), func(ctx context.Context, p auth.Principal, a memberArgs) (any, error) {
				deleted, err := s.Revoke(ctx, p, a.Space, a.Name)
				if err != nil {
					return nil, err
				}
				return map[string]bool{"deleted": deleted}, nil
			}),
	}
}

func spaceOnly() map[string]any { return map[string]any{"space": spaceProp()} }
func memberProps() map[string]any {
	return map[string]any{"space": spaceProp(), "name": str(nameDesc("Member agent"))}
}
func spaceReq() []string  { return []string{"space"} }
func memberReq() []string { return []string{"space", "name"} }
