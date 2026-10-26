package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
)

// Noticer takes the caller's pending "You have mail" notice (inbox.Service).
type Noticer interface {
	TakeNotice(ctx context.Context, p auth.Principal) (inbox.Notice, bool, error)
}

// addHint appends the "You have mail" text block to res when p's mailbox scope holds an
// unnoticed edge. The tool already ran, so a failed notice query is logged, not returned:
// the edge stays unnoticed and a later result carries the hint.
func addHint(ctx context.Context, notices Noticer, p auth.Principal, res *mcp.CallToolResult) {
	n, due, err := notices.TakeNotice(ctx, p)
	if err != nil {
		slog.ErrorContext(ctx, "taking mail notice failed", "agent_id", p.AgentID, "err", err)
		return
	}
	if due {
		res.Content = append(res.Content, &mcp.TextContent{Text: hintText(n)})
	}
}

// hintText is the one-line hint of contracts §9.
func hintText(n inbox.Notice) string {
	critical := ""
	if n.Critical > 0 {
		critical = fmt.Sprintf(" (%d critical)", n.Critical)
	}
	return fmt.Sprintf("[agent-bus] You have mail: %d waiting in %s%s. "+
		"Call bus_inbox_read, handle the entries, then bus_inbox_ack {upTo}.",
		n.Waiting, strings.Join(n.Spaces, ", "), critical)
}
