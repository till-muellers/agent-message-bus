package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
)

// addHint appends the "You have mail" text block to res when p's mailbox scope holds an
// unnoticed edge (inbox.NoticeAfter).
func addHint(ctx context.Context, notices inbox.Noticer, p auth.Principal, res *mcp.CallToolResult) {
	if n, due := inbox.NoticeAfter(ctx, notices, p); due {
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
