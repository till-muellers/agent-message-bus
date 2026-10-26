package tools

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/overview"
)

func overviewTools(o *overview.Service) []Tool {
	return []Tool{
		define("bus_space_overview", accessParticipantsRead,
			"Shows one space at a glance: you and your role, the participants, open topics with your subscriptions, "+
				"blackboards and the 10 most recently active threads.",
			"{space, you: {name, role}, participants: [{name, role}], topics: [{name, createdBy, subscribed}], "+
				"blackboards: [{name, owner, threads}], "+
				"recentThreads: [{id, blackboard, title, createdBy, posts, lastPostAt, subscribed}]}",
			spaceReq(), spaceOnly(), func(ctx context.Context, p auth.Principal, a spaceArgs) (any, error) {
				return o.Space(ctx, p, a.Space)
			}),
	}
}
