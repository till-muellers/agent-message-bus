package server_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// openInvite is one row of GET /web/api/spaces/{space}/invites.
type openInvite struct {
	ID, Name, InvitedBy, ExpiresAt string
}

// inviteWorld is a webWorld where leader "lead" created an invite code for "new" in demo.
func inviteWorld(t *testing.T) (*webWorld, string) {
	t.Helper()
	w := newWebWorld(t, false)
	var inv struct{ Code string }
	callTool(t, w.api, w.lead, "bus_invite_member", `{"space":"demo","name":"new"}`, &inv)
	return w, inv.Code
}

// openInvites lists the open invites of demo through the GUI as admin ops.
func (w *webWorld) openInvites(t *testing.T) []openInvite {
	t.Helper()
	status, body, _ := w.send(t, http.MethodGet, "/web/api/spaces/demo/invites", "", w.open(t, w.ops), "")
	var out []openInvite
	if err := json.Unmarshal([]byte(body), &out); status != http.StatusOK || err != nil {
		t.Fatalf("list invites: %d %s %v", status, body, err)
	}
	return out
}

// accepts reports whether code is still redeemable.
func (w *webWorld) accepts(t *testing.T, code string) bool {
	t.Helper()
	status, _ := do(t, w.api, http.MethodPost, "/v1/invites/accept", code, "")
	return status == http.StatusOK
}

var inviteID = regexp.MustCompile(`^[0-9a-f]{16}$`)

func TestWebInvites(t *testing.T) {
	t.Parallel()
	w, code := inviteWorld(t)
	list := w.openInvites(t)
	if len(list) != 1 || !inviteID.MatchString(list[0].ID) || list[0].Name != "new" || list[0].InvitedBy != "lead" ||
		list[0].ExpiresAt == "" {
		t.Fatalf("open invites = %+v", list)
	}
	s := w.open(t, w.lead)
	path := "/web/api/spaces/demo/invites/" + list[0].ID
	for _, want := range []string{`{"cancelled":true}`, `{"cancelled":false}`} {
		status, body, h := w.send(t, http.MethodDelete, path, "", s, s.csrf)
		if status != http.StatusOK || strings.TrimSpace(body) != want || h.Get("Cache-Control") != "no-store" {
			t.Fatalf("cancel = %d %s, want %s", status, body, want)
		}
	}
	if w.accepts(t, code) {
		t.Fatal("the cancelled code is still accepted")
	}
}

// inviteAccessCase is one request by the session of as ("" = no session) with csrf "own",
// "foreign" or "" (none).
type inviteAccessCase struct {
	name, as, method, path, csrf string
	status                       int
	want                         string
}

func TestWebInvitesAccess(t *testing.T) {
	t.Parallel()
	const list = "/web/api/spaces/demo/invites"
	for _, tc := range []inviteAccessCase{
		{"member lists", "worker", http.MethodGet, list, "", http.StatusForbidden, "only leaders may list or cancel invites"},
		{"member cancels", "worker", http.MethodDelete, list + "/", "own", http.StatusForbidden,
			"only leaders may list or cancel invites"},
		{"leader of another space", "lead2", http.MethodGet, list, "", http.StatusForbidden, "not a participant of space demo"},
		{"unknown space", "ops", http.MethodGet, "/web/api/spaces/nope/invites", "", http.StatusNotFound,
			"space nope not found"},
		{"list without a session", "", http.MethodGet, list, "", http.StatusUnauthorized, "session required"},
		{"cancel without a session", "", http.MethodDelete, list + "/", "", http.StatusUnauthorized, "session required"},
		{"cancel without a CSRF token", "ops", http.MethodDelete, list + "/", "", http.StatusForbidden, "invalid CSRF token"},
		{"cancel with a foreign CSRF token", "ops", http.MethodDelete, list + "/", "foreign", http.StatusForbidden,
			"invalid CSRF token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runInviteAccess(t, tc)
		})
	}
}

func runInviteAccess(t *testing.T, tc inviteAccessCase) {
	t.Helper()
	w, code := inviteWorld(t)
	path := tc.path
	if strings.HasSuffix(path, "/") {
		path += w.openInvites(t)[0].ID
	}
	var lead2 struct{ Token string }
	callTool(t, w.api, adminToken, "bus_mint_leader", `{"name":"lead2"}`, &lead2)
	tokens := map[string]string{"ops": w.ops, "worker": w.worker, "lead2": lead2.Token}
	s := w.open(t, tokens[tc.as])
	csrf := map[string]string{"own": s.csrf, "foreign": w.open(t, w.ops).csrf}[tc.csrf]
	status, body, _ := w.send(t, tc.method, path, "", s, csrf)
	if status != tc.status || !strings.Contains(body, tc.want) {
		t.Fatalf("%s %s = %d %s, want %d %q", tc.method, path, status, body, tc.status, tc.want)
	}
	if !w.accepts(t, code) {
		t.Fatal("a refused request cancelled the code")
	}
}

// TestCancelLogLine swaps the default logger, so it must not run in parallel.
func TestCancelLogLine(t *testing.T) {
	buf := captureLog(t)
	w, _ := inviteWorld(t)
	s := w.open(t, w.ops)
	path := "/web/api/spaces/demo/invites/" + w.openInvites(t)[0].ID
	for range 2 {
		w.send(t, http.MethodDelete, path, "", s, s.csrf)
	}
	w.send(t, http.MethodDelete, "/web/api/spaces/demo/invites/nope", "", s, s.csrf)
	want := []string{"cancel-invite demo new"}
	if got := mutationLines(t, buf, w.ops); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("web mutation lines = %q, want %q", got, want)
	}
}

func TestNoCancelTool(t *testing.T) {
	t.Parallel()
	w := newWebWorld(t, false)
	status, body := do(t, w.api, http.MethodGet, "/v1/tools", adminToken, "")
	if status != http.StatusOK || strings.Contains(string(body), "cancel_invite") {
		t.Fatalf("GET /v1/tools = %d, want 200 without a cancel tool: %s", status, body)
	}
}
