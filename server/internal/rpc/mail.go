package rpc

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
)

// success is the POST /v1/tools success body; Mail is present only on the first result
// after an unnoticed edge in the caller's mailbox scope (contracts §9).
type success struct {
	Result any           `json:"result"`
	Mail   *inbox.Notice `json:"mail,omitempty"`
}

// mailFor returns p's pending notice (inbox.NoticeAfter), or nil.
func mailFor(ctx context.Context, notices inbox.Noticer, p auth.Principal) *inbox.Notice {
	n, due := inbox.NoticeAfter(ctx, notices, p)
	if !due {
		return nil
	}
	return &n
}
