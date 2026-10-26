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
		define("bus_whoami", "Any token; no side effects.",
			"Returns the caller's kind, name and the spaces it participates in.",
			"{kind, name?, space?, spaces: [{name, role}]}",
			nil, nil, func(ctx context.Context, p auth.Principal, _ noArgs) (any, error) {
				return s.Whoami(ctx, p)
			}),
		define("bus_list_spaces", "Leaders and admins see all spaces, members only their own; no side effects.",
			"Lists spaces with their leaders, joined admins, member count and whether the caller joined.",
			"[{name, leaders, admins, members, joined}]",
			nil, nil, func(ctx context.Context, p auth.Principal, _ noArgs) (any, error) {
				return list(s.List(ctx, p))
			}),
		define("bus_list_participants", accessParticipantsRead, "Lists the space's admins, leaders and members, in that order.",
			"[{name, role}]",
			spaceReq(), spaceOnly(), func(ctx context.Context, p auth.Principal, a spaceArgs) (any, error) {
				return list(s.Participants(ctx, p, a.Space))
			}),
	}
}

func spaceLifecycleTools(s *spaces.Service) []Tool {
	return []Tool{
		spaceOp("bus_create_space", accessLeaders,
			"Creates the space with its all topic and joins the caller as leader (an admin with role admin).",
			s, (*spaces.Service).Create),
		spaceOp("bus_join_space", accessLeaders,
			"Joins an existing space as a leader (an admin with role admin); a space can have several leaders.",
			s, (*spaces.Service).Join),
		spaceOp("bus_leave_space", "Participants.",
			"Leaves the space and drops the caller's inbox entries and subscriptions there; the last leader cannot leave. "+
				"Pitfall: when a member leaves, a member's boards, inbox entries, subscriptions and token are deleted.",
			s, (*spaces.Service).Leave),
		spaceOp("bus_delete_space", "Leaders of the space once no other leader remains, and admins.",
			"Deletes the space and everything in it, including member tokens. "+
				"Pitfall: an admin may delete any space, even one with other leaders.", s, (*spaces.Service).Delete),
	}
}

// spaceOp binds fn to s only when the tool is called.
func spaceOp(name, access, desc string, s *spaces.Service,
	fn func(*spaces.Service, context.Context, auth.Principal, string) error) Tool {
	return define(name, access, desc, okResult, spaceReq(), spaceOnly(), func(ctx context.Context, p auth.Principal, a spaceArgs) (any, error) {
		return done(fn(s, ctx, p, a.Space))
	})
}

func memberTools(s *spaces.Service) []Tool {
	return []Tool{
		define("bus_revoke_member", accessSpaceLeaders,
			"Removes the member, if present, with its tokens, inbox entries and blackboards, and cancels its pending "+
				"invite codes for the space; idempotent, deleted reports whether anything was removed. "+
				"Pitfall: leaders cannot revoke leaders, but an admin may also revoke leaders, the last leader included.",
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
