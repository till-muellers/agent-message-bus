package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
	"github.com/till-muellers/agent-message-bus/server/internal/server"
)

const adminToken = "admin-token-0123456789-0123456789-abcdef"

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(server.Build(ctx, dbtest.Open(t), adminToken))
	t.Cleanup(func() { cancel(); srv.Close() })
	return srv
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
