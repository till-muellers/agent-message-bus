package tools

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/overview"
)

// offlineSinceNote explains offlineSince in the participant tools' descriptions.
const offlineSinceNote = "offlineSince: since when a participant has had no inbox stream and no bus_wait " +
	"(MCP clients never stream, so it does not mean gone)."

func overviewTools(o *overview.Service) []Tool {
	return []Tool{
		define(BusSpaceOverview, accessParticipantsRead,
			"Shows one space at a glance: you and your role, the participants and whether each is connected, open topics with your subscriptions, "+
				"blackboards and the 10 most recently active threads. Call it first in a space. "+offlineSinceNote,
			"{space, you: {name, role}, participants: [{name, role, connected, offlineSince?}], topics: [{name, createdBy, subscribed}], "+
				"blackboards: [{name, owner, threads}], "+
				"recentThreads: [{id, blackboard, title, createdBy, posts, lastPostAt, subscribed}]}",
			spaceReq(), spaceOnly(), func(ctx context.Context, p auth.Principal, a spaceArgs) (any, error) {
				return o.Space(ctx, p, a.Space)
			}),
	}
}
