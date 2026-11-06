// Package mcpserver exposes the tool registry over MCP streamable HTTP.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/guide"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/tools"
)

// errInternal is the protocol error of a failure without apperr kind; the detail stays in the log.
var errInternal = errors.New("internal error")

// NewHandler returns a stateless MCP HTTP handler serving reg behind auth.Service.Middleware,
// so /mcp parses and rejects bearer tokens like every other route. A successful tool result
// gains a trailing "You have mail" text block when notices has one.
// Every client receives the agent guide as the server instructions.
func NewHandler(reg *tools.Registry, a *auth.Service, notices inbox.Noticer) http.Handler {
	srv := mcp.NewServer(&mcp.Implementation{Name: "agent-message-bus", Version: "1"},
		&mcp.ServerOptions{Instructions: guide.Text})
	for _, t := range reg.List() {
		srv.AddTool(&mcp.Tool{Name: t.Name, Description: t.Description, InputSchema: t.Schema}, handler(reg, t.Name, notices))
	}
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, streamableOptions())
	mw := sdkauth.RequireBearerToken(verifier, &sdkauth.RequireBearerTokenOptions{AllowMissingExpiration: true})
	return a.Middleware(mw(h))
}

// streamableOptions turns the SDK's DNS-rebinding guard off. The guard answers 403 to a request
// that arrives on a loopback address with a Host other than localhost, which is every request
// of a same-host TLS reverse proxy forwarding the public Host; /v1 never had it, so behind such
// a proxy only /mcp broke. It protects nothing here: it runs after the bearer check, and a
// rebinding page cannot present a token (the browser never adds the bearer header by itself, no
// cookie or other ambient credential authenticates on this listener, and the server sends no
// CORS headers), while whoever holds a token calls /v1 or /mcp directly.
func streamableOptions() *mcp.StreamableHTTPOptions {
	return &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, DisableLocalhostProtection: true}
}

func handler(reg *tools.Registry, name string, notices inbox.Noticer) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		p, ok := principalOf(req.Extra.TokenInfo)
		if !ok {
			return nil, errors.New("missing principal")
		}
		var args json.RawMessage
		if len(req.Params.Arguments) > 0 {
			args = req.Params.Arguments
		}
		out, err := reg.Call(ctx, p, name, args)
		if err != nil {
			return errorResult(ctx, err)
		}
		res, err := successResult(out)
		if err != nil {
			return errorResult(ctx, err)
		}
		addHint(ctx, notices, p, res)
		return res, nil
	}
}

// errorResult is the isError result "<kind>: <message>". An error without kind is logged
// and fails the call as a protocol error "internal error", never with its detail, like the
// HTTP transports' 500.
func errorResult(ctx context.Context, err error) (*mcp.CallToolResult, error) {
	var ae *apperr.Error
	if !errors.As(err, &ae) {
		slog.ErrorContext(ctx, "internal error", "err", err)
		return nil, errInternal
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%s: %s", ae.Kind, ae.Msg)}},
	}, nil
}

func successResult(out any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("marshaling result: %w", err)
	}
	res := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}
	if len(b) > 0 && b[0] == '{' {
		res.StructuredContent = json.RawMessage(b)
	}
	return res, nil
}
