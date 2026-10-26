package rpc

import (
	"context"
	"log/slog"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
)

// Noticer takes the caller's pending "You have mail" notice (inbox.Service).
type Noticer interface {
	TakeNotice(ctx context.Context, p auth.Principal) (inbox.Notice, bool, error)
}

// success is the POST /v1/tools success body; Mail is present only on the first result
// after an unnoticed edge in the caller's mailbox scope (contracts §9).
type success struct {
	Result any           `json:"result"`
	Mail   *inbox.Notice `json:"mail,omitempty"`
}

// mailFor returns p's pending notice, or nil. The tool already ran, so a failed notice
// query is logged, not returned: the edge stays unnoticed and a later result carries it.
func mailFor(ctx context.Context, notices Noticer, p auth.Principal) *inbox.Notice {
	n, due, err := notices.TakeNotice(ctx, p)
	if err != nil {
		slog.ErrorContext(ctx, "taking mail notice failed", "agent_id", p.AgentID, "err", err)
		return nil
	}
	if !due {
		return nil
	}
	return &n
}
