package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
	"github.com/till-muellers/agent-message-bus/server/internal/server"
)

// webWorld is a bus with its API and GUI handlers on two test servers, seeded per
// the gui-access.feature background: leader "lead" with space "demo" and member "worker".
type webWorld struct {
	api, web     *httptest.Server
	lead, worker string
	ops          string // token of minted admin "ops"
}

func newWebWorld(t *testing.T, tls bool) *webWorld {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	h, err := server.Build(ctx, dbtest.Open(t), adminToken, func() {})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	w := &webWorld{api: httptest.NewServer(h.API)}
	if tls {
		w.web = httptest.NewTLSServer(h.Web)
	} else {
		w.web = httptest.NewServer(h.Web)
	}
	t.Cleanup(func() { cancel(); w.api.Close(); w.web.Close() })
	var lead, ops struct{ Token string }
	callTool(t, w.api, adminToken, "bus_mint_leader", `{"name":"lead"}`, &lead)
	callTool(t, w.api, lead.Token, "bus_create_space", `{"space":"demo"}`, nil)
	callTool(t, w.api, adminToken, "bus_mint_admin", `{"name":"ops"}`, &ops)
	w.lead, w.ops = lead.Token, ops.Token
	w.worker = invite(t, w.api, lead.Token, "demo", "worker")
	return w
}

// webDo sends method path to the GUI server with an optional raw Authorization header
// and session cookie.
func (w *webWorld) webDo(t *testing.T, method, path, authz string, c *http.Cookie) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, w.web.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	if c != nil {
		req.AddCookie(c)
	}
	res, err := w.web.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func sessionCookie(res *http.Response) *http.Cookie {
	for _, c := range res.Cookies() {
		if c.Name == "amb_session" {
			return c
		}
	}
	return nil
}

type sessionBody struct {
	Kind, Name, Space, CSRF string
}

// signIn signs in with token and returns the session cookie and body; it fails unless 200.
func (w *webWorld) signIn(t *testing.T, token string) (*http.Cookie, sessionBody) {
	t.Helper()
	res := w.webDo(t, http.MethodPost, "/web/session", "Bearer "+token, nil)
	body := readBody(t, res)
	c := sessionCookie(res)
	if res.StatusCode != http.StatusOK || c == nil {
		t.Fatalf("sign in: %d %s cookie %v", res.StatusCode, body, c)
	}
	var out sessionBody
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	return c, out
}

// status returns the status code and body of a GUI request with cookie c.
func (w *webWorld) status(t *testing.T, method, path string, c *http.Cookie) (int, string) {
	t.Helper()
	res := w.webDo(t, method, path, "", c)
	return res.StatusCode, readBody(t, res)
}

func TestAPIListenerHasNoWeb(t *testing.T) {
	t.Parallel()
	w := newWebWorld(t, false)
	for _, path := range []string{"/web/", "/web/session", "/web/api/state", "/web/assets/app.js"} {
		if status, body := do(t, w.api, http.MethodGet, path, "", ""); status != http.StatusNotFound {
			t.Fatalf("API GET %s = %d %s, want 404", path, status, body)
		}
	}
	if status, body := do(t, w.api, http.MethodPost, "/web/session", adminToken, ""); status != http.StatusNotFound {
		t.Fatalf("API POST /web/session = %d %s, want 404", status, body)
	}
	if status, body := w.status(t, http.MethodGet, "/healthz", nil); status != http.StatusNotFound {
		t.Fatalf("GUI GET /healthz = %d %s, want 404", status, body)
	}
}

func TestSignIn(t *testing.T) {
	t.Parallel()
	w := newWebWorld(t, false)
	ok := []struct {
		name, token string
		want        sessionBody
	}{
		{"bootstrap admin", adminToken, sessionBody{Kind: "admin", Name: "admin"}},
		{"minted admin", w.ops, sessionBody{Kind: "admin", Name: "ops"}},
		{"leader", w.lead, sessionBody{Kind: "leader", Name: "lead"}},
		{"member", w.worker, sessionBody{Kind: "member", Name: "worker", Space: "demo"}},
	}
	for _, tc := range ok {
		t.Run(tc.name, func(t *testing.T) {
			c, got := w.signIn(t, tc.token)
			if len(got.CSRF) != 43 || got.Kind != tc.want.Kind || got.Name != tc.want.Name || got.Space != tc.want.Space {
				t.Fatalf("body %+v, want %+v with a 43-char csrf", got, tc.want)
			}
			if c.Path != "/web" || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.MaxAge != 28800 || c.Secure {
				t.Fatalf("cookie %+v", c)
			}
			if strings.Contains(c.Value, tc.token) {
				t.Fatal("cookie holds the token")
			}
		})
	}
	bad := []struct{ name, authz, secret, wantMsg string }{
		{"unknown", "Bearer amb_nope", "amb_nope", "invalid token"},
		{"empty", "", "", "bearer token required"},
		{"wrong kind", "Basic " + adminToken, adminToken, "bearer token required"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			res := w.webDo(t, http.MethodPost, "/web/session", tc.authz, nil)
			body := readBody(t, res)
			if res.StatusCode != http.StatusUnauthorized || !strings.Contains(body, `"code":"unauthenticated"`) ||
				!strings.Contains(body, tc.wantMsg) {
				t.Fatalf("got %d %s", res.StatusCode, body)
			}
			if res.Header.Get("Set-Cookie") != "" {
				t.Fatalf("cookie set: %s", res.Header.Get("Set-Cookie"))
			}
			if tc.secret != "" && strings.Contains(body, tc.secret) {
				t.Fatal("token echoed")
			}
		})
	}
}

func TestSignInSecureCookieOnTLS(t *testing.T) {
	t.Parallel()
	w := newWebWorld(t, true)
	if c, _ := w.signIn(t, adminToken); !c.Secure {
		t.Fatalf("cookie over TLS is not Secure: %+v", c)
	}
}

func TestSessionRestore(t *testing.T) {
	t.Parallel()
	w := newWebWorld(t, false)
	c, first := w.signIn(t, w.worker)
	status, body := w.status(t, http.MethodGet, "/web/session", c)
	var got sessionBody
	if err := json.Unmarshal([]byte(body), &got); status != http.StatusOK || err != nil || got != first {
		t.Fatalf("restore: %d %s, want %+v", status, body, first)
	}
	status, body = w.status(t, http.MethodGet, "/web/session", nil)
	if status != http.StatusUnauthorized || !strings.Contains(body, "session required") {
		t.Fatalf("restore without cookie: %d %s", status, body)
	}
	forged := &http.Cookie{Name: "amb_session", Value: "forged"}
	if status, body = w.status(t, http.MethodGet, "/web/session", forged); status != http.StatusUnauthorized {
		t.Fatalf("restore with a forged cookie: %d %s", status, body)
	}
}

// expectEnded asserts that c's session answers 401 "invalid token" once, then "session required".
func (w *webWorld) expectEnded(t *testing.T, c *http.Cookie) {
	t.Helper()
	for _, want := range []string{"invalid token", "session required"} {
		status, body := w.status(t, http.MethodGet, "/web/session", c)
		if status != http.StatusUnauthorized || !strings.Contains(body, want) {
			t.Fatalf("ended session: %d %s, want 401 %q", status, body, want)
		}
	}
}

func (w *webWorld) expectAlive(t *testing.T, c *http.Cookie) {
	t.Helper()
	if status, body := w.status(t, http.MethodGet, "/web/session", c); status != http.StatusOK {
		t.Fatalf("live session: %d %s", status, body)
	}
}

func TestSessionRevalidation(t *testing.T) {
	t.Parallel()
	t.Run("rotate ends the bootstrap admin's sessions only", func(t *testing.T) {
		w := newWebWorld(t, false)
		boot, _ := w.signIn(t, adminToken)
		ops, _ := w.signIn(t, w.ops)
		next := strings.Repeat("r", 40)
		if status, body := do(t, w.api, http.MethodPost, "/v1/admin/admin-token", adminToken, `{"token":"`+next+`"}`); status != http.StatusOK {
			t.Fatalf("rotate: %d %s", status, body)
		}
		w.expectEnded(t, boot)
		w.expectAlive(t, ops)
	})
	t.Run("revoking an admin ends its sessions", func(t *testing.T) {
		w := newWebWorld(t, false)
		ops, _ := w.signIn(t, w.ops)
		callTool(t, w.api, adminToken, "bus_revoke_admin", `{"name":"ops"}`, nil)
		w.expectEnded(t, ops)
	})
	t.Run("revoking a member ends its sessions", func(t *testing.T) {
		w := newWebWorld(t, false)
		worker, _ := w.signIn(t, w.worker)
		callTool(t, w.api, w.lead, "bus_revoke_member", `{"space":"demo","name":"worker"}`, nil)
		if status, body := w.status(t, http.MethodGet, "/web/api/state", worker); status != http.StatusUnauthorized ||
			!strings.Contains(body, "invalid token") {
			t.Fatalf("revoked member on /web/api/state: %d %s", status, body)
		}
	})
	t.Run("signing out drops the session", func(t *testing.T) {
		w := newWebWorld(t, false)
		boot, _ := w.signIn(t, adminToken)
		res := w.webDo(t, http.MethodDelete, "/web/session", "", boot)
		body := readBody(t, res)
		cleared := sessionCookie(res)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, `"ok":true`) || cleared == nil || cleared.MaxAge >= 0 {
			t.Fatalf("sign out: %d %s cookie %+v", res.StatusCode, body, cleared)
		}
		if status, body := w.status(t, http.MethodGet, "/web/session", boot); status != http.StatusUnauthorized ||
			!strings.Contains(body, "session required") {
			t.Fatalf("after sign out: %d %s", status, body)
		}
	})
}

func TestWebReadOnly(t *testing.T) {
	t.Parallel()
	w := newWebWorld(t, false)
	c, _ := w.signIn(t, adminToken)
	routes := map[string][]string{
		"/web/":              {http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		"/web/assets/app.js": {http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		"/web/api/state":     {http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		"/web/session":       {http.MethodPut, http.MethodPatch},
	}
	for path, methods := range routes {
		for _, m := range methods {
			if status, body := w.status(t, m, path, c); status != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s = %d %s, want 405", m, path, status, body)
			}
		}
	}
}

func TestSessionCookieNotBearer(t *testing.T) {
	t.Parallel()
	for _, who := range []string{"leader", "minted admin"} {
		t.Run(who, func(t *testing.T) {
			w := newWebWorld(t, false)
			token := map[string]string{"leader": w.lead, "minted admin": w.ops}[who]
			c, _ := w.signIn(t, token)
			req, err := http.NewRequest(http.MethodGet, w.api.URL+"/v1/inbox/stream", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.AddCookie(c)
			res, err := w.api.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			if body := readBody(t, res); res.StatusCode != http.StatusUnauthorized {
				t.Fatalf("stream with the cookie only: %d %s", res.StatusCode, body)
			}
			status, _, closeStream := openStream(t, w.api, token)
			defer closeStream()
			if status != http.StatusOK {
				t.Fatalf("own stream with the bearer: %d", status)
			}
		})
	}
}
