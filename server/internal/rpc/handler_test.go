package rpc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
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

type reply struct {
	Result json.RawMessage `json:"result"`
	Error  struct {
		Code string `json:"code"`
	} `json:"error"`
}

func call(t *testing.T, srv *httptest.Server, token, tool, args string) (int, reply) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/tools/"+tool, bytes.NewBufferString(args))
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
	var out reply
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, out
}

func mustCall(t *testing.T, srv *httptest.Server, token, tool, args string, into any) {
	t.Helper()
	status, out := call(t, srv, token, tool, args)
	if status != http.StatusOK {
		t.Fatalf("%s: status %d code %s", tool, status, out.Error.Code)
	}
	if into != nil {
		if err := json.Unmarshal(out.Result, into); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHappyPath(t *testing.T) {
	srv := newServer(t)
	var lead struct{ Token string }
	mustCall(t, srv, adminToken, "bus_mint_leader", `{"name":"boss"}`, &lead)
	mustCall(t, srv, lead.Token, "bus_create_space", `{"space":"proj"}`, nil)
	var mem struct{ Name, Space, Token string }
	mustCall(t, srv, lead.Token, "bus_invite_member", `{"space":"proj","name":"worker"}`, &mem)
	mustCall(t, srv, lead.Token, "bus_create_topic", `{"space":"proj","topic":"news"}`, nil)
	mustCall(t, srv, mem.Token, "bus_send_topic", `{"space":"proj","topic":"news","body":"hi"}`, nil)
	var entries []inbox.Entry
	mustCall(t, srv, lead.Token, "bus_inbox_read", `{}`, &entries)
	if len(entries) != 1 || entries[0].Body != "hi" || entries[0].From != "worker" || entries[0].Topic != "news" {
		t.Fatalf("got %+v", entries)
	}
}

func TestErrors(t *testing.T) {
	srv := newServer(t)
	cases := []struct {
		name, token, tool, args string
		status                  int
		code                    string
	}{
		{"no token", "", "bus_whoami", "", http.StatusUnauthorized, "unauthenticated"},
		{"unknown tool", adminToken, "bus_nope", "", http.StatusNotFound, "not_found"},
		{"unknown arg", adminToken, "bus_mint_leader", `{"name":"x","bogus":1}`, http.StatusBadRequest, "invalid"},
		{"missing arg", adminToken, "bus_mint_leader", `{}`, http.StatusBadRequest, "invalid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, out := call(t, srv, c.token, c.tool, c.args)
			if status != c.status || out.Error.Code != c.code {
				t.Fatalf("got %d %s", status, out.Error.Code)
			}
		})
	}
}

func TestListTools(t *testing.T) {
	srv := newServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/tools", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var got []struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		InputSchema map[string]any `json:"inputSchema"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || len(got) != 29 || got[0].InputSchema["type"] != "object" {
		t.Fatalf("status %d, %d tools", res.StatusCode, len(got))
	}
}
