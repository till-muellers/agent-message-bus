package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// session is a signed-in GUI browser: its cookie and its CSRF token.
type session struct {
	cookie *http.Cookie
	csrf   string
}

// open signs in with token; an empty token is no session.
func (w *webWorld) open(t *testing.T, token string) session {
	t.Helper()
	if token == "" {
		return session{}
	}
	c, body := w.signIn(t, token)
	return session{cookie: c, csrf: body.CSRF}
}

// send makes a GUI request with s's cookie, header X-CSRF-Token csrf (when set) and body.
func (w *webWorld) send(t *testing.T, method, path, body string, s session, csrf string) (int, string, http.Header) {
	t.Helper()
	req, err := http.NewRequest(method, w.web.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if s.cookie != nil {
		req.AddCookie(s.cookie)
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	res, err := w.web.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, readBody(t, res), res.Header
}

type adminRow struct {
	Name      string
	Bootstrap bool
	Tokens    int
	Spaces    []string
}

// admins lists the admins through the API with the bootstrap token.
func (w *webWorld) admins(t *testing.T) map[string]adminRow {
	t.Helper()
	var rows []adminRow
	callTool(t, w.api, adminToken, "bus_list_admins", `{}`, &rows)
	out := map[string]adminRow{}
	for _, r := range rows {
		out[r.Name] = r
	}
	return out
}

// authenticates reports whether token still works on the API.
func (w *webWorld) authenticates(t *testing.T, token string) bool {
	t.Helper()
	status, _ := do(t, w.api, http.MethodPost, "/v1/tools/bus_whoami", token, `{}`)
	return status == http.StatusOK
}

// adminCase is one admin-area request by the session of as ("" = no session); check, when
// set, inspects the outcome with that session s and the response body.
type adminCase struct {
	name, as, method, path, body string
	status                       int
	want                         string
	check                        func(t *testing.T, w *webWorld, s session, body string)
}

func adminCases() []adminCase {
	return []adminCase{
		{name: "admin session lists the admins", as: "ops", method: http.MethodGet, path: "/web/api/admins",
			status: http.StatusOK, check: checkList},
		{name: "mint shows the token once", as: "ops", method: http.MethodPost, path: "/web/api/admins",
			body: `{"name":"web"}`, status: http.StatusOK, want: `"name":"web"`, check: checkMinted},
		{name: "invalid name is refused", as: "admin", method: http.MethodPost, path: "/web/api/admins",
			body: `{"name":"Bad Name"}`, status: http.StatusBadRequest, want: `name must match ^[a-z0-9][a-z0-9_-]{0,62}$`,
			check: noMint},
		{name: "bootstrap name cannot be minted", as: "admin", method: http.MethodPost, path: "/web/api/admins",
			body: `{"name":"admin"}`, status: http.StatusConflict,
			want: "admin is the bootstrap admin; its token is the instance admin token", check: noMint},
		{name: "a name of another kind cannot be minted", as: "admin", method: http.MethodPost, path: "/web/api/admins",
			body: `{"name":"lead"}`, status: http.StatusConflict, want: "agent lead is a leader agent", check: noMint},
		{name: "revoke ends the admin and its sessions", as: "ops", method: http.MethodPost,
			path: "/web/api/admins/ops/revoke", status: http.StatusOK, want: `"deleted":true`, check: checkRevoked},
		{name: "bootstrap admin cannot be revoked", as: "admin", method: http.MethodPost,
			path: "/web/api/admins/admin/revoke", status: http.StatusForbidden,
			want: "admin is the bootstrap admin; rotate its token with agent-message-bus rotate-token"},
		{name: "unknown admin reports nothing removed", as: "admin", method: http.MethodPost,
			path: "/web/api/admins/ghost/revoke", status: http.StatusOK, want: `"deleted":false`},
		{name: "no session cannot list", method: http.MethodGet, path: "/web/api/admins",
			status: http.StatusUnauthorized, want: "session required"},
		{name: "no session cannot mint", method: http.MethodPost, path: "/web/api/admins", body: `{"name":"web"}`,
			status: http.StatusUnauthorized, want: "session required", check: noMint},
		{name: "no session cannot revoke", method: http.MethodPost, path: "/web/api/admins/ops/revoke",
			status: http.StatusUnauthorized, want: "session required", check: opsAlive},
		{name: "leader session sees no admin list", as: "lead", method: http.MethodGet, path: "/web/api/admins",
			status: http.StatusForbidden, want: "admin token required"},
		{name: "leader session cannot mint", as: "lead", method: http.MethodPost, path: "/web/api/admins",
			body: `{"name":"web"}`, status: http.StatusForbidden, want: "admin token required", check: noMint},
		{name: "leader session cannot revoke", as: "lead", method: http.MethodPost, path: "/web/api/admins/ops/revoke",
			status: http.StatusForbidden, want: "admin token required", check: opsAlive},
		{name: "member session sees no admin list", as: "worker", method: http.MethodGet, path: "/web/api/admins",
			status: http.StatusForbidden, want: "admin token required"},
		{name: "member session cannot mint", as: "worker", method: http.MethodPost, path: "/web/api/admins",
			body: `{"name":"web"}`, status: http.StatusForbidden, want: "admin token required", check: noMint},
	}
}

func TestWebAdmins(t *testing.T) {
	t.Parallel()
	for _, tc := range adminCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newWebWorld(t, false)
			tokens := map[string]string{"admin": adminToken, "ops": w.ops, "lead": w.lead, "worker": w.worker}
			s := w.open(t, tokens[tc.as])
			status, body, h := w.send(t, tc.method, tc.path, tc.body, s, s.csrf)
			if status != tc.status || !strings.Contains(body, tc.want) {
				t.Fatalf("%s %s = %d %s, want %d %q", tc.method, tc.path, status, body, tc.status, tc.want)
			}
			if cc := h.Get("Cache-Control"); cc != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", cc)
			}
			if tc.check != nil {
				tc.check(t, w, s, body)
			}
		})
	}
}

// checkList asserts the GUI list matches bus_list_admins: bootstrap admin and minted ops.
func checkList(t *testing.T, _ *webWorld, _ session, body string) {
	t.Helper()
	var rows []adminRow
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatalf("list body %s: %v", body, err)
	}
	if len(rows) != 2 || rows[0].Name != "admin" || !rows[0].Bootstrap || rows[0].Tokens != 0 ||
		rows[1].Name != "ops" || rows[1].Bootstrap || rows[1].Tokens != 1 {
		t.Fatalf("admin list = %+v", rows)
	}
}

// checkMinted asserts the shown token signs in as admin "web" and is never listed again.
func checkMinted(t *testing.T, w *webWorld, s session, body string) {
	t.Helper()
	var minted struct{ Name, Token string }
	if err := json.Unmarshal([]byte(body), &minted); err != nil || !strings.HasPrefix(minted.Token, "amb_admin_") {
		t.Fatalf("mint body %s: %v", body, err)
	}
	if _, who := w.signIn(t, minted.Token); who.Kind != "admin" || who.Name != "web" {
		t.Fatalf("minted token signs in as %+v", who)
	}
	status, list, _ := w.send(t, http.MethodGet, "/web/api/admins", "", s, "")
	if status != http.StatusOK || strings.Contains(list, minted.Token) || !strings.Contains(list, `"name":"web"`) {
		t.Fatalf("reloaded list: %d %s", status, list)
	}
	if got := w.admins(t)["web"].Tokens; got != 1 {
		t.Fatalf("web tokens = %d, want 1", got)
	}
}

// noMint asserts the admins are still exactly the world's admin and ops.
func noMint(t *testing.T, w *webWorld, _ session, _ string) {
	t.Helper()
	if got := w.admins(t); len(got) != 2 {
		t.Fatalf("admins = %+v, want admin and ops only", got)
	}
}

// opsAlive asserts admin "ops" still authenticates.
func opsAlive(t *testing.T, w *webWorld, _ session, _ string) {
	t.Helper()
	if !w.authenticates(t, w.ops) {
		t.Fatal("ops no longer authenticates")
	}
}

// checkRevoked asserts ops's token and its GUI sessions are gone, and ops stays listed with 0 tokens.
func checkRevoked(t *testing.T, w *webWorld, s session, _ string) {
	t.Helper()
	if w.authenticates(t, w.ops) {
		t.Fatal("revoked ops still authenticates")
	}
	if row, ok := w.admins(t)["ops"]; !ok || row.Tokens != 0 {
		t.Fatalf("ops after revoke = %+v (listed %v)", row, ok)
	}
	if status, body, _ := w.send(t, http.MethodGet, "/web/api/state", "", s, ""); status != http.StatusUnauthorized ||
		!strings.Contains(body, "invalid token") {
		t.Fatalf("revoked admin's session on /web/api/state: %d %s", status, body)
	}
}

func TestCSRF(t *testing.T) {
	t.Parallel()
	routes := []struct{ name, path, body string }{
		{"mint", "/web/api/admins", `{"name":"web"}`},
		{"revoke", "/web/api/admins/ops/revoke", ""},
	}
	for _, rt := range routes {
		for _, header := range []string{"missing", "foreign session", "wrong value"} {
			t.Run(rt.name+" "+header, func(t *testing.T) {
				t.Parallel()
				w := newWebWorld(t, false)
				s := w.open(t, adminToken)
				csrf := map[string]string{"missing": "", "foreign session": w.open(t, adminToken).csrf,
					"wrong value": strings.Repeat("x", 43)}[header]
				status, body, _ := w.send(t, http.MethodPost, rt.path, rt.body, s, csrf)
				if status != http.StatusForbidden || !strings.Contains(body, `"code":"forbidden"`) ||
					!strings.Contains(body, "invalid CSRF token") {
					t.Fatalf("POST %s = %d %s, want 403 invalid CSRF token", rt.path, status, body)
				}
				noMint(t, w, s, "")
				opsAlive(t, w, s, "")
			})
		}
	}
}
