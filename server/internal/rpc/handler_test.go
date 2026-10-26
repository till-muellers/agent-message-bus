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
	h, err := server.Build(ctx, dbtest.Open(t), adminToken, func() {})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(func() { cancel(); srv.Close() })
	return srv
}

type reply struct {
	Result json.RawMessage `json:"result"`
	Error  struct {
		Code string `json:"code"`
	} `json:"error"`
}

// post sends body to path with an optional bearer and decodes the JSON response into out.
func post(t *testing.T, srv *httptest.Server, path, bearer, body string, out any) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		t.Fatal(err)
	}
	return res.StatusCode
}

func call(t *testing.T, srv *httptest.Server, token, tool, args string) (int, reply) {
	t.Helper()
	var out reply
	status := post(t, srv, "/v1/tools/"+tool, token, args, &out)
	return status, out
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
	lead := setUpSpace(t, srv)
	var inv struct{ Name, Space, Code, ExpiresAt string }
	mustCall(t, srv, lead, "bus_invite_member", `{"space":"proj","name":"worker","task":"say hi"}`, &inv)
	var mem struct{ Name, Space, Token, Task string }
	if status := post(t, srv, "/v1/invites/accept", inv.Code, "", &mem); status != http.StatusOK ||
		mem.Name != "worker" || mem.Space != "proj" || mem.Task != "say hi" {
		t.Fatalf("accept: status %d %+v", status, mem)
	}
	mustCall(t, srv, lead, "bus_create_topic", `{"space":"proj","topic":"news"}`, nil)
	mustCall(t, srv, mem.Token, "bus_send_topic", `{"space":"proj","topic":"news","body":"hi"}`, nil)
	var entries []inbox.Entry
	mustCall(t, srv, lead, "bus_inbox_read", `{}`, &entries)
	if len(entries) != 1 || entries[0].Body != "hi" || entries[0].From != "worker" || entries[0].Topic != "news" {
		t.Fatalf("got %+v", entries)
	}
}

func TestRevokeCancelsInvite(t *testing.T) {
	srv := newServer(t)
	lead := setUpSpace(t, srv)
	var inv struct{ Code string }
	mustCall(t, srv, lead, "bus_invite_member", `{"space":"proj","name":"worker"}`, &inv)
	for _, want := range []bool{true, false} {
		var got struct{ Deleted bool }
		mustCall(t, srv, lead, "bus_revoke_member", `{"space":"proj","name":"worker"}`, &got)
		if got.Deleted != want {
			t.Fatalf("deleted = %v, want %v", got.Deleted, want)
		}
	}
	var out reply
	if status := post(t, srv, "/v1/invites/accept", inv.Code, "", &out); status != http.StatusUnauthorized {
		t.Fatalf("accept after revoke: status %d", status)
	}
}

// setUpSpace mints leader boss, creates space proj and returns the leader token.
func setUpSpace(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	var lead struct{ Token string }
	mustCall(t, srv, adminToken, "bus_mint_leader", `{"name":"boss"}`, &lead)
	mustCall(t, srv, lead.Token, "bus_create_space", `{"space":"proj"}`, nil)
	return lead.Token
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
	if res.StatusCode != http.StatusOK || len(got) != 32 || got[0].InputSchema["type"] != "object" {
		t.Fatalf("status %d, %d tools", res.StatusCode, len(got))
	}
}
