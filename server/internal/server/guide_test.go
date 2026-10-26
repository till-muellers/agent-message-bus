package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/till-muellers/agent-message-bus/server/internal/guide"
)

// guideTokens returns a server with leader "lead", space "alpha" and member "worker",
// and a token per kind.
func guideTokens(t *testing.T) (*httptest.Server, map[string]string) {
	t.Helper()
	srv := newServer(t)
	var lead struct{ Token string }
	callTool(t, srv, adminToken, "bus_mint_leader", `{"name":"lead"}`, &lead)
	callTool(t, srv, lead.Token, "bus_create_space", `{"space":"alpha"}`, nil)
	return srv, map[string]string{"admin": adminToken, "leader": lead.Token,
		"member": invite(t, srv, lead.Token, "alpha", "worker")}
}

// getGuide sends GET /v1/guide (with a bearer unless token is "") and returns status,
// Content-Type and body.
func getGuide(t *testing.T, srv *httptest.Server, token string) (int, string, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/v1/guide", nil)
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
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, res.Header.Get("Content-Type"), body
}

func TestGetGuide(t *testing.T) {
	srv, tokens := guideTokens(t)
	for kind, tok := range tokens {
		t.Run(kind, func(t *testing.T) {
			status, ctype, body := getGuide(t, srv, tok)
			if status != http.StatusOK || ctype != "text/markdown; charset=utf-8" || string(body) != guide.Text {
				t.Fatalf("status %d content-type %q body %q", status, ctype, body)
			}
		})
	}
	t.Run("no bearer", func(t *testing.T) {
		status, _, body := getGuide(t, srv, "")
		var out struct {
			Error struct{ Code, Message string } `json:"error"`
		}
		if err := json.Unmarshal(body, &out); status != http.StatusUnauthorized || err != nil ||
			out.Error.Code != "unauthenticated" || out.Error.Message != "bearer token required" {
			t.Fatalf("status %d body %s err %v", status, body, err)
		}
	})
}

func TestMCPInstructions(t *testing.T) {
	srv, tokens := guideTokens(t)
	for kind, tok := range tokens {
		t.Run(kind, func(t *testing.T) {
			c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
			s, err := c.Connect(t.Context(), &mcp.StreamableClientTransport{
				Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: bearer{tok}},
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			if got := s.InitializeResult().Instructions; got != guide.Text {
				t.Fatalf("instructions = %q, want the embedded guide", got)
			}
		})
	}
}
