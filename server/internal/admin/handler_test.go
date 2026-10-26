package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

type fakeSetter struct{ got string }

func (f *fakeSetter) SetAdminToken(tok string) error {
	if len(tok) < 32 {
		return apperr.New(apperr.Invalid, "admin token must be at least 32 characters")
	}
	f.got = tok
	return nil
}

// fakeAuthn stores the principal named by the bearer token: "bootstrap" (the instance
// admin token), "admin" (a minted admin) or "leader".
func fakeAuthn(next http.Handler) http.Handler {
	principals := map[string]auth.Principal{
		"bootstrap": {Kind: auth.Admin, AgentName: auth.BootstrapName, Bootstrap: true},
		"admin":     {Kind: auth.Admin, AgentName: "ops"},
		"leader":    {Kind: auth.Leader, AgentName: "lead"},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := principals[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	})
}

func TestMount(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("n", 32)
	tests := []struct {
		name, path, remote, token, body string
		wantStatus                      int
		wantBody                        string
		wantShutdown                    bool
		wantToken                       string
	}{
		{name: "shutdown", path: "/v1/admin/shutdown", remote: "127.0.0.1:5", token: "admin", wantStatus: 200, wantBody: `"ok":true`, wantShutdown: true},
		{name: "shutdown ipv6 loopback", path: "/v1/admin/shutdown", remote: "[::1]:5", token: "admin", wantStatus: 200, wantShutdown: true},
		{name: "remote", path: "/v1/admin/shutdown", remote: "10.0.0.2:5", token: "admin", wantStatus: 403, wantBody: "admin endpoints are only available from localhost"},
		{name: "leader", path: "/v1/admin/shutdown", remote: "127.0.0.1:5", token: "leader", wantStatus: 403, wantBody: "admin token required"},
		{name: "rotate", path: "/v1/admin/admin-token", remote: "127.0.0.1:5", token: "bootstrap", body: `{"token":"` + long + `"}`, wantStatus: 200, wantToken: long},
		{name: "rotate short", path: "/v1/admin/admin-token", remote: "127.0.0.1:5", token: "bootstrap", body: `{"token":"x"}`, wantStatus: 400, wantBody: "at least 32"},
		{name: "rotate bad json", path: "/v1/admin/admin-token", remote: "127.0.0.1:5", token: "bootstrap", body: `{`, wantStatus: 400},
		{name: "rotate remote", path: "/v1/admin/admin-token", remote: "192.168.1.1:5", token: "bootstrap", body: `{"token":"` + long + `"}`, wantStatus: 403},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mux, ts, down := http.NewServeMux(), &fakeSetter{}, false
			Mount(mux, fakeAuthn, ts, func() { down = true })
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.RemoteAddr = tc.remote
			req.Header.Set("Authorization", "Bearer "+tc.token)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus || !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("got %d %s", rec.Code, rec.Body)
			}
			if down != tc.wantShutdown || ts.got != tc.wantToken {
				t.Fatalf("shutdown %v token %q", down, ts.got)
			}
		})
	}
}

// postAdmin sends a POST to path from remote with the fake token and reports the response,
// whether shutdown ran and the token handed to the setter.
func postAdmin(t *testing.T, path, remote, token, body string) (*httptest.ResponseRecorder, bool, string) {
	t.Helper()
	mux, ts, down := http.NewServeMux(), &fakeSetter{}, false
	Mount(mux, fakeAuthn, ts, func() { down = true })
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, strings.NewReader(body))
	req.RemoteAddr = remote
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec, down, ts.got
}

func TestAdminTokenSwapBootstrapOnly(t *testing.T) {
	t.Parallel()
	body := `{"token":"` + strings.Repeat("n", 32) + `"}`
	rec, _, got := postAdmin(t, "/v1/admin/admin-token", "127.0.0.1:5", "admin", body)
	if rec.Code != http.StatusForbidden || got != "" ||
		!strings.Contains(rec.Body.String(), "only the bootstrap admin may replace the instance admin token") {
		t.Fatalf("minted admin swap: %d %s (token %q)", rec.Code, rec.Body, got)
	}
	if rec, _, got := postAdmin(t, "/v1/admin/admin-token", "127.0.0.1:5", "bootstrap", body); rec.Code != http.StatusOK || got == "" {
		t.Fatalf("bootstrap swap: %d %s", rec.Code, rec.Body)
	}
}

func TestShutdownAnyAdmin(t *testing.T) {
	t.Parallel()
	for _, token := range []string{"bootstrap", "admin"} {
		if rec, down, _ := postAdmin(t, "/v1/admin/shutdown", "127.0.0.1:5", token, ""); rec.Code != http.StatusOK || !down {
			t.Fatalf("%s shutdown: %d %s (down %v)", token, rec.Code, rec.Body, down)
		}
	}
}
