package mcpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
		{Meta: tools.Meta{Name: "whoami", Description: "who", Schema: schema},
			Call: func(_ context.Context, p auth.Principal, _ json.RawMessage) (any, error) {
				return map[string]any{"kind": string(p.Kind)}, nil
			}},
		{Meta: tools.Meta{Name: "forbid", Description: "no", Schema: schema},
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

func setup(t *testing.T, reg *tools.Registry) (string, string) {
	t.Helper()
	ctx := context.Background()
	a := auth.NewService(dbtest.Open(t), "admin-tok", time.Now)
	root, err := a.Authenticate(ctx, "admin-tok")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := a.MintLeader(ctx, root, "lead")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(mcpserver.NewHandler(reg, a, noNotice{}))
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
	url, tok := setup(t, testRegistry())
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

func TestInternalFailureIsAProtocolErrorWithoutDetail(t *testing.T) {
	crash := tools.Tool{Meta: tools.Meta{Name: "crash", Description: "fails", Schema: map[string]any{"type": "object"}},
		Call: func(context.Context, auth.Principal, json.RawMessage) (any, error) {
			return nil, errors.New("secret db detail")
		}}
	url, tok := setup(t, tools.NewRegistry([]tools.Tool{crash}))
	s, err := connect(context.Background(), url, tok)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	_, err = s.CallTool(context.Background(), &mcp.CallToolParams{Name: "crash"})
	if err == nil || !strings.Contains(err.Error(), "internal error") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v, want a protocol error \"internal error\" without the detail", err)
	}
}

// /mcp parses the header with auth.BearerToken and rejects with the §0 JSON envelope, like
// the /v1 routes.
func TestBearerAuth(t *testing.T) {
	url, tok := setup(t, testRegistry())
	tests := []struct {
		name, header string
		wantStatus   int
		wantBody     string
	}{
		{"no token", "", 401, `{"error":{"code":"unauthenticated","message":"bearer token required"}}`},
		{"bad token", "Bearer amb_nope", 401, `{"error":{"code":"unauthenticated","message":"invalid token"}}`},
		{"other scheme", "Basic " + tok, 401, `{"error":{"code":"unauthenticated","message":"bearer token required"}}`},
		{"lower-case scheme", "bearer " + tok, 200, `"result"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, ctype, body := postPing(t, url, tc.header, "")
			if status != tc.wantStatus || !strings.Contains(body, tc.wantBody) {
				t.Fatalf("got %d %s, want %d %s", status, body, tc.wantStatus, tc.wantBody)
			}
			if status == 401 && ctype != "application/json" {
				t.Fatalf("Content-Type %q, want application/json", ctype)
			}
		})
	}
}

// postPing sends a JSON-RPC ping to url with the Authorization header (none when empty) and
// the Host header host (the URL's own host when empty).
func postPing(t *testing.T, url, header, host string) (status int, contentType, body string) {
	t.Helper()
	ping := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, ping)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	if host != "" {
		req.Host = host
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, res.Header.Get("Content-Type"), string(b)
}
