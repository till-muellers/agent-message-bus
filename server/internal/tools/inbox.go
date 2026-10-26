package tools

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
)

type readArgs struct {
	After int64 `json:"after"`
	Limit int   `json:"limit"`
}

type ackArgs struct {
	UpTo int64 `json:"upTo"`
}

func inboxTools(ib *inbox.Service) []Tool {
	return []Tool{
		define("bus_inbox_read", accessInbox,
			"Returns inbox entries after the given id without removing them; members see only their space. "+
				"Pitfalls: omp sessions receive entries automatically and need not poll; it never blocks and reading does not acknowledge.",
			"[{id, space, type, from, critical, topic?, blackboard?, threadId?, threadTitle?, body?, createdAt}]",
			nil, map[string]any{"after": integer("Return entries with id greater than this; default 0."), "limit": integer("Maximum entries, 1..500; default 100.")},
			func(ctx context.Context, p auth.Principal, a readArgs) (any, error) {
				return list(ib.Read(ctx, p, a.After, a.Limit))
			}),
		define("bus_inbox_ack", accessInbox,
			"Permanently deletes the caller's inbox entries with id up to upTo and returns how many were removed. "+
				"Pitfall: it deletes every entry with id up to upTo, so never acknowledge past what was handled.",
			"{acked}",
			[]string{"upTo"}, map[string]any{"upTo": integer("Highest entry id to acknowledge.")},
			func(ctx context.Context, p auth.Principal, a ackArgs) (any, error) {
				n, err := ib.Ack(ctx, p, a.UpTo)
				if err != nil {
					return nil, err
				}
				return map[string]int64{"acked": n}, nil
			}),
	}
}
