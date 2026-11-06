package mcpserver_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A TLS reverse proxy on the same host reaches the loopback listener with the public Host.
// The httptest server listens on 127.0.0.1, so these are loopback connections with foreign
// Host headers: /mcp answers like /v1 (the bearer check decides, never the Host).
func TestForeignHostOnLoopbackBind(t *testing.T) {
	url, tok := setup(t, testRegistry())
	tests := []struct {
		name, header, host string
		wantStatus         int
		wantBody           string
	}{
		{"public host", "Bearer " + tok, "bus.example.com", 200, `"result"`},
		{"public host with port", "Bearer " + tok, "example.com:8443", 200, `"result"`},
		{"localhost", "Bearer " + tok, "localhost:8080", 200, `"result"`},
		{"public host without token", "", "bus.example.com", 401, `"code":"unauthenticated"`},
		{"public host with bad token", "Bearer amb_nope", "bus.example.com", 401, `"message":"invalid token"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, _, body := postPing(t, url, tc.header, tc.host)
			if status != tc.wantStatus || !strings.Contains(body, tc.wantBody) {
				t.Fatalf("got %d %s, want %d %s", status, body, tc.wantStatus, tc.wantBody)
			}
		})
	}
}

// hostRewrite sends every request with the Host header host, as a proxy forwarding the public
// Host does.
type hostRewrite struct{ token, host string }

func (h hostRewrite) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Host = h.host
	return bearer{h.token}.RoundTrip(r)
}

func TestMCPClientBehindProxyForwardingPublicHost(t *testing.T) {
	url, tok := setup(t, testRegistry())
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	s, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: url, HTTPClient: &http.Client{Transport: hostRewrite{tok, "bus.example.com"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "whoami"})
	if err != nil {
		t.Fatal(err)
	}
	if got := text(res); !strings.Contains(got, `"kind":"leader"`) {
		t.Fatalf("whoami = %s", got)
	}
}
