package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
	"github.com/till-muellers/agent-message-bus/server/internal/server"
)

const adminToken = "admin-token-0123456789-0123456789-abcdef"

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(mustBuild(ctx, t, func() {}))
	t.Cleanup(func() { cancel(); srv.Close() })
	return srv
}

// mustBuild builds the server on a fresh database and fails the test on error.
func mustBuild(ctx context.Context, t *testing.T, shutdown func()) http.Handler {
	t.Helper()
	h, err := server.Build(ctx, dbtest.Open(t), adminToken, shutdown)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return h
}

func do(t *testing.T, srv *httptest.Server, method, path, token, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	out, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, out
}

func TestHealthz(t *testing.T) {
	status, body := do(t, newServer(t), http.MethodGet, "/healthz", "", "")
	if status != http.StatusOK || string(body) != "ok" {
		t.Fatalf("got %d %q", status, body)
	}
}

func TestUnauthenticated(t *testing.T) {
	srv := newServer(t)
	cases := []struct {
		name, method, path string
		wantCode           bool
	}{
		{"list tools", http.MethodGet, "/v1/tools", true},
		{"call tool", http.MethodPost, "/v1/tools/bus_whoami", true},
		{"inbox stream", http.MethodGet, "/v1/inbox/stream", true},
		{"mcp", http.MethodPost, "/mcp", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, body := do(t, srv, c.method, c.path, "", "")
			if status != http.StatusUnauthorized {
				t.Fatalf("status %d", status)
			}
			if !c.wantCode {
				return
			}
			var out struct {
				Error struct{ Code string } `json:"error"`
			}
			if err := json.Unmarshal(body, &out); err != nil || out.Error.Code != "unauthenticated" {
				t.Fatalf("body %s err %v", body, err)
			}
		})
	}
}

func callTool(t *testing.T, srv *httptest.Server, token, tool, args string, into any) {
	t.Helper()
	status, body := do(t, srv, http.MethodPost, "/v1/tools/"+tool, token, args)
	if status != http.StatusOK {
		t.Fatalf("%s: status %d body %s", tool, status, body)
	}
	var out struct{ Result json.RawMessage }
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if into != nil {
		if err := json.Unmarshal(out.Result, into); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCreateSpaceAddsAllTopic(t *testing.T) {
	srv := newServer(t)
	var lead struct{ Token string }
	callTool(t, srv, adminToken, "bus_mint_leader", `{"name":"boss"}`, &lead)
	callTool(t, srv, lead.Token, "bus_create_space", `{"space":"proj"}`, nil)
	var topics []struct {
		Name      string `json:"name"`
		CreatedBy string `json:"createdBy"`
	}
	callTool(t, srv, lead.Token, "bus_list_topics", `{"space":"proj"}`, &topics)
	if len(topics) != 1 || topics[0].Name != "all" || topics[0].CreatedBy != "boss" {
		t.Fatalf("got %+v", topics)
	}
}

// openStream opens GET /v1/inbox/stream as token; closing it drops the connection.
func openStream(t *testing.T, srv *httptest.Server, token string) (int, []byte, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/v1/inbox/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	closeStream := func() { cancel(); _ = res.Body.Close() }
	if res.StatusCode == http.StatusOK {
		return res.StatusCode, nil, closeStream
	}
	defer closeStream()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, body, func() {}
}

func TestInboxStreamOnePerAgent(t *testing.T) {
	srv := newServer(t)
	var lead struct{ Token string }
	callTool(t, srv, adminToken, "bus_mint_leader", `{"name":"boss"}`, &lead)
	status, _, closeFirst := openStream(t, srv, lead.Token)
	if status != http.StatusOK {
		t.Fatalf("first stream status %d", status)
	}
	status, body, _ := openStream(t, srv, lead.Token)
	if status != http.StatusConflict || !strings.Contains(string(body), `"code":"conflict"`) ||
		!strings.Contains(string(body), "agent boss already has an active inbox stream") {
		t.Fatalf("second stream: %d %s", status, body)
	}
	closeFirst()
	deadline := time.Now().Add(5 * time.Second)
	for {
		status, _, closeNext := openStream(t, srv, lead.Token)
		closeNext()
		if status == http.StatusOK {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("stream after disconnect still %d", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAdminEndpoints(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	down := make(chan struct{}, 1)
	srv := httptest.NewServer(mustBuild(ctx, t, func() { down <- struct{}{} }))
	t.Cleanup(func() { cancel(); srv.Close() })
	next := strings.Repeat("r", 40)
	if status, body := do(t, srv, http.MethodPost, "/v1/admin/admin-token", adminToken, `{"token":"`+next+`"}`); status != http.StatusOK {
		t.Fatalf("rotate: %d %s", status, body)
	}
	if status, _ := do(t, srv, http.MethodGet, "/v1/tools", adminToken, ""); status != http.StatusUnauthorized {
		t.Fatalf("old token: %d", status)
	}
	if status, _ := do(t, srv, http.MethodGet, "/v1/tools", next, ""); status != http.StatusOK {
		t.Fatalf("new token: %d", status)
	}
	if status, _ := do(t, srv, http.MethodPost, "/v1/admin/shutdown", next, ""); status != http.StatusOK {
		t.Fatalf("shutdown: %d", status)
	}
	select {
	case <-down:
	default:
		t.Fatal("shutdown callback not called")
	}
}

func TestBuildBootstrapError(t *testing.T) {
	t.Parallel()
	conn := dbtest.Open(t)
	dbtest.SeedAgent(t, conn, 1, "admin", "leader")
	_, err := server.Build(context.Background(), conn, adminToken, func() {})
	if err == nil || !strings.Contains(err.Error(), "agent admin is a leader agent") {
		t.Fatalf("Build err = %v, want the bootstrap conflict", err)
	}
}

func TestHealthzProtocolHeader(t *testing.T) {
	t.Parallel()
	srv := newServer(t)
	res, err := srv.Client().Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if got := res.Header.Get("Agent-Bus-Protocol"); got != "2" {
		t.Fatalf("Agent-Bus-Protocol = %q, want 2", got)
	}
}

func TestHTTPServerKeepsLongCalls(t *testing.T) {
	t.Parallel()
	srv := server.NewHTTPServer(http.NotFoundHandler())
	if srv.WriteTimeout != 0 || srv.ReadTimeout != 0 || srv.IdleTimeout != 0 || srv.ReadHeaderTimeout != 10*time.Second {
		t.Fatalf("server timeouts = write %v read %v idle %v header %v; want only a 10s header timeout",
			srv.WriteTimeout, srv.ReadTimeout, srv.IdleTimeout, srv.ReadHeaderTimeout)
	}
}
