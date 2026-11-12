package tools

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
)

type readArgs struct {
	After int64 `json:"after"`
	Limit *int  `json:"limit"`
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
		define(BusInboxRead, accessInbox,
			"Returns your inbox entries after id `after` without removing them; members see only their space. "+
				"Never blocks (use bus_wait) and does not acknowledge. omp sessions receive entries by themselves and need not call it.",
			"[{id, space, type, from, critical, class, topic?, blackboard?, threadId?, threadTitle?, postId?, body?, createdAt}]",
			nil, map[string]any{"after": integer("Return entries with id greater than this; default 0."), "limit": integer("Maximum entries, 1..500; default 100.")},
			func(ctx context.Context, p auth.Principal, a readArgs) (any, error) {
				limit, err := inbox.PageSize(a.Limit)
				if err != nil {
					return nil, err
				}
				es, err := ib.Read(ctx, p, a.After, limit)
				if err != nil {
					return nil, err
				}
				return list(es, ib.MarkNoticed(ctx, p, es))
			}),
		define(BusInboxAck, accessInbox,
			"Deletes your inbox entries with id up to upTo and returns how many. "+
				"Acknowledge only what you handled; acknowledging everything lets the next mail notify again.",
			"{acked}",
			[]string{"upTo"}, map[string]any{"upTo": integer("Highest entry id to acknowledge.")},
			func(ctx context.Context, p auth.Principal, a ackArgs) (any, error) {
				n, err := ib.Ack(ctx, p, a.UpTo)
				if err != nil {
					return nil, err
				}
				return map[string]int64{"acked": n}, nil
			}),
		define(BusWait, accessInbox,
			"Blocks until unacknowledged entries match (at most 100, id order) or timeoutSeconds (1..300, default 30) pass, "+
				"then returns them, or [] on timeout; `from` and `threadId` narrow it. Does not acknowledge (omp does). "+
				"In omp a queued aside, steer or critical entry ends the wait early: continue with that entry.",
			"[{id, space, type, from, critical, class, topic?, blackboard?, threadId?, threadTitle?, postId?, body?, createdAt}]",
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
