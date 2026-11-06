package inbox

import (
	"context"
	"log/slog"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
)

// Noticer takes the caller's pending "You have mail" notice (Service.TakeNotice).
type Noticer interface {
	TakeNotice(ctx context.Context, p auth.Principal) (Notice, bool, error)
}

// NoticeAfter returns p's pending notice once a tool call has succeeded, for the transport
// to attach to the result. The call already ran, so a failed notice query is logged, not
// returned: the edge stays unnoticed and a later result carries it. A caller that went
// away (ctx done) gets no notice and no log line: nothing would reach it.
func NoticeAfter(ctx context.Context, n Noticer, p auth.Principal) (Notice, bool) {
	if ctx.Err() != nil {
		return Notice{}, false
	}
	notice, due, err := n.TakeNotice(ctx, p)
	if err != nil {
		if ctx.Err() == nil {
			slog.ErrorContext(ctx, "taking mail notice failed", "agent_id", p.AgentID, "err", err)
		}
		return Notice{}, false
	}
	return notice, due
}
