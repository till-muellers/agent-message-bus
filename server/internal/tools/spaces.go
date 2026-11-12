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

// offlineSinceNote explains offlineSince in the descriptions of the tools that list participants.
const offlineSinceNote = "offlineSince: since when a participant has had no inbox stream and no bus_wait; " +
	"clients that never stream or wait always have it, so it does not mean gone."

func spaceTools(s *spaces.Service) []Tool {
	return append(spaceIdentityTools(s), append(spaceLifecycleTools(s), memberTools(s)...)...)
}

func spaceIdentityTools(s *spaces.Service) []Tool {
	return []Tool{
		define(BusWhoami, "Any token; no side effects.",
			"Returns the caller's kind, name and the spaces it participates in.",
			"{kind, name?, space?, spaces: [{name, role}]}",
			nil, nil, func(ctx context.Context, p auth.Principal, _ noArgs) (any, error) {
				return s.Whoami(ctx, p)
			}),
		define(BusListSpaces, "Leaders and admins see all spaces, members only their own; no side effects.",
			"Lists spaces with their leaders, joined admins, member count and whether the caller joined.",
			"[{name, leaders, admins, members, joined}]",
			nil, nil, func(ctx context.Context, p auth.Principal, _ noArgs) (any, error) {
				return list(s.List(ctx, p))
			}),
		define(BusListParticipants, accessParticipantsRead, "Lists the space's admins, leaders and members, in that order, with whether each is connected. "+
			offlineSinceNote,
			"[{name, role, connected, offlineSince?}]",
			spaceReq(), spaceOnly(), func(ctx context.Context, p auth.Principal, a spaceArgs) (any, error) {
				return list(s.Participants(ctx, p, a.Space))
			}),
	}
}

func spaceLifecycleTools(s *spaces.Service) []Tool {
	return []Tool{
		spaceOp(BusCreateSpace, accessLeaders,
			"Creates the space with its `all` topic and joins you as leader (role admin for an admin).",
			newSpaceProps(), s, (*spaces.Service).Create),
		spaceOp(BusJoinSpace, accessLeaders,
			"Joins an existing space as leader (role admin for an admin); a space can have several leaders.",
			spaceOnly(), s, (*spaces.Service).Join),
		spaceOp(BusLeaveSpace, "Participants.",
			"Leaves the space and drops your inbox entries and subscriptions there. A member's token and boards are deleted too: "+
				"post your state to a leader's board first. The last leader cannot leave while others remain; "+
				"the last participant leaving deletes the space.",
			spaceOnly(), s, (*spaces.Service).Leave),
		spaceOp(BusDeleteSpace, "Leaders of the space once no other leader remains, and admins.",
			"Deletes the space and everything in it, including member tokens. "+
				"An admin may delete any space, even one with other leaders.", spaceOnly(), s, (*spaces.Service).Delete),
	}
}

// spaceOp binds fn to s only when the tool is called.
func spaceOp(name, access, desc string, props map[string]any, s *spaces.Service,
	fn func(*spaces.Service, context.Context, auth.Principal, string) error) Tool {
	return define(name, access, desc, okResult, spaceReq(), props, func(ctx context.Context, p auth.Principal, a spaceArgs) (any, error) {
		return done(fn(s, ctx, p, a.Space))
	})
}

func memberTools(s *spaces.Service) []Tool {
	return []Tool{
		define(BusRevokeMember, accessSpaceLeaders,
			"Removes the member with its tokens, inbox entries and blackboards and cancels its pending invite codes; "+
				"idempotent, `deleted` says whether anything was removed. Leaders cannot revoke leaders; "+
				"an admin can, even the last leader, and the space is deleted when nobody remains.",
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

func spaceOnly() map[string]any     { return map[string]any{"space": spaceProp()} }
func newSpaceProps() map[string]any { return map[string]any{"space": newNameProp("Space")} }
func memberProps() map[string]any {
	return map[string]any{"space": spaceProp(), "name": nameProp("Member agent")}
}
func spaceReq() []string  { return []string{"space"} }
func memberReq() []string { return []string{"space", "name"} }
