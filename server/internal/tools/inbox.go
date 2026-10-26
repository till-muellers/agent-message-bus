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

type waitArgs struct {
	TimeoutSeconds *int   `json:"timeoutSeconds"`
	After          int64  `json:"after"`
	From           string `json:"from"`
	ThreadID       int64  `json:"threadId"`
}

func inboxTools(ib *inbox.Service) []Tool {
	return []Tool{
		define("bus_inbox_read", accessInbox,
			"Returns inbox entries after the given id without removing them; members see only their space. "+
				"Pitfalls: omp sessions receive entries automatically and need not poll; it never blocks (use bus_wait to wait) and reading does not acknowledge.",
			"[{id, space, type, from, critical, class, topic?, blackboard?, threadId?, threadTitle?, body?, createdAt}]",
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
		define("bus_wait", accessInbox,
			"Waits for mail: returns the matching unacknowledged entries of the caller's scope (at most 100, id order) "+
				"as soon as there are any. Pitfall: it blocks until mail arrives or timeoutSeconds (1..300, default 30) pass, "+
				"returns [] on timeout, filters with from/threadId and does not acknowledge (omp acknowledges for you).",
			"[{id, space, type, from, critical, class, topic?, blackboard?, threadId?, threadTitle?, body?, createdAt}]",
			nil, waitProps(), func(ctx context.Context, p auth.Principal, a waitArgs) (any, error) {
				timeout, err := inbox.WaitTimeout(a.TimeoutSeconds)
				if err != nil {
					return nil, err
				}
				return list(ib.Wait(ctx, p, inbox.WaitQuery{After: a.After, From: a.From, ThreadID: a.ThreadID, Timeout: timeout}))
			}),
	}
}

func waitProps() map[string]any {
	return map[string]any{
		"timeoutSeconds": integer("Longest wait in seconds, 1..300; default 30."),
		"after":          integer("Only entries with id greater than this; default 0."),
		"from":           str("Only entries from this sender name."),
		"threadId":       integer("Only entries of this thread."),
	}
}
