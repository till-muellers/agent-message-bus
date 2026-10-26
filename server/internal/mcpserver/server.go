// Package mcpserver exposes the tool registry over MCP streamable HTTP.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/tools"
)

// NewHandler returns a bearer-authenticated, stateless MCP HTTP handler serving reg. A
// successful tool result gains a trailing "You have mail" text block when notices has one.
func NewHandler(reg *tools.Registry, a *auth.Service, notices Noticer) http.Handler {
	srv := mcp.NewServer(&mcp.Implementation{Name: "agent-message-bus", Version: "1"}, nil)
	for _, t := range reg.List() {
		srv.AddTool(&mcp.Tool{Name: t.Name, Description: t.Description, InputSchema: t.Schema}, handler(t, notices))
	}
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	mw := sdkauth.RequireBearerToken(verifier(a), &sdkauth.RequireBearerTokenOptions{AllowMissingExpiration: true})
	return mw(h)
}

func handler(t tools.Tool, notices Noticer) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		p, ok := principalOf(req.Extra.TokenInfo)
		if !ok {
			return nil, errors.New("missing principal")
		}
		var args json.RawMessage
		if len(req.Params.Arguments) > 0 {
			args = req.Params.Arguments
		}
		out, err := t.Call(ctx, p, args)
		if err != nil {
			return errorResult(err)
		}
		res, err := successResult(out)
		if err != nil {
			return nil, err
		}
		addHint(ctx, notices, p, res)
		return res, nil
	}
}

func errorResult(err error) (*mcp.CallToolResult, error) {
	var ae *apperr.Error
	if !errors.As(err, &ae) {
		return nil, err
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
