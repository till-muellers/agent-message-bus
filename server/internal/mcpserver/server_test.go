package mcpserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/mcpserver"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
	"github.com/till-muellers/agent-message-bus/server/internal/tools"
)

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if b.token != "" {
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func testRegistry() *tools.Registry {
	schema := map[string]any{"type": "object"}
	return tools.NewRegistry([]tools.Tool{
		{Name: "whoami", Description: "who", Schema: schema,
			Call: func(_ context.Context, p auth.Principal, _ json.RawMessage) (any, error) {
				return map[string]any{"kind": string(p.Kind)}, nil
			}},
		{Name: "forbid", Description: "no", Schema: schema,
			Call: func(context.Context, auth.Principal, json.RawMessage) (any, error) {
				return nil, apperr.New(apperr.Forbidden, "nope")
			}},
	})
}

// noNotice is a mailbox scope without pending mail.
type noNotice struct{}

func (noNotice) TakeNotice(context.Context, auth.Principal) (inbox.Notice, bool, error) {
	return inbox.Notice{}, false, nil
}

func setup(t *testing.T) (string, string) {
	t.Helper()
	ctx := context.Background()
	a := auth.NewService(dbtest.Open(t), "admin-tok", time.Now)
	tok, err := a.MintLeader(ctx, auth.Principal{Kind: auth.Admin}, "lead")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(mcpserver.NewHandler(testRegistry(), a, noNotice{}))
	t.Cleanup(srv.Close)
	return srv.URL, tok
}

func connect(ctx context.Context, url, token string) (*mcp.ClientSession, error) {
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	return c.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: url, HTTPClient: &http.Client{Transport: bearer{token}},
	}, nil)
}

func mustConnect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	url, tok := setup(t)
	s, err := connect(context.Background(), url, tok)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestListToolsSorted(t *testing.T) {
	res, err := mustConnect(t).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != 2 || res.Tools[0].Name != "forbid" || res.Tools[1].Name != "whoami" {
		t.Fatalf("tools = %+v", res.Tools)
	}
}

func text(r *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range r.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestCallTool(t *testing.T) {
	s := mustConnect(t)
	cases := []struct {
		name, want string
		isErr      bool
	}{
		{"whoami", `"kind":"leader"`, false},
		{"forbid", "forbidden: nope", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: c.name})
			if err != nil {
				t.Fatal(err)
			}
			if r.IsError != c.isErr || !strings.Contains(text(r), c.want) {
				t.Fatalf("isErr=%v text=%q", r.IsError, text(r))
			}
		})
	}
}

func TestNoTokenRejected(t *testing.T) {
	url, _ := setup(t)
	s, err := connect(context.Background(), url, "")
	if err == nil {
		_, err = s.ListTools(context.Background(), nil)
		_ = s.Close()
	}
	if err == nil {
		t.Fatal("expected failure without token")
	}
}
