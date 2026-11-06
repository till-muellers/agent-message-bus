package server_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// streamUntilEnd opens token's inbox stream and returns a channel that receives every line
// the server sends, closed when the server ends the stream.
func streamUntilEnd(t *testing.T, srv *httptest.Server, token string) <-chan string {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/v1/inbox/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := srv.Client().Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("opening the stream: %v %v", res, err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	return lines
}

// endsWithout waits until the stream ends and fails if a line contained secret.
func endsWithout(t *testing.T, lines <-chan string, secret string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				return
			}
			if strings.Contains(line, secret) {
				t.Fatalf("the stream of the dead token got %q", line)
			}
		case <-deadline:
			t.Fatal("the stream of the dead token is still open")
		}
	}
}

func TestStreamOfADeadMemberTokenEndsBeforeTheReinvitedMembersMail(t *testing.T) {
	t.Parallel()
	for name, kill := range map[string]func(t *testing.T, w mailWorld){
		"revoked": func(t *testing.T, w mailWorld) {
			callTool(t, w.srv, w.lead, "bus_revoke_member", `{"space":"alpha","name":"worker"}`, nil)
		},
		"left": func(t *testing.T, w mailWorld) {
			callTool(t, w.srv, w.worker, "bus_leave_space", `{"space":"alpha"}`, nil)
		},
		"space deleted and created again": func(t *testing.T, w mailWorld) {
			callTool(t, w.srv, w.lead, "bus_delete_space", `{"space":"alpha"}`, nil)
			callTool(t, w.srv, w.lead, "bus_create_space", `{"space":"alpha"}`, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newMailWorld(t)
			old := streamUntilEnd(t, w.srv, w.worker)
			kill(t, w)
			fresh := inviteWithTask(t, w, "secret task")
			endsWithout(t, old, "secret")
			status, _, closeStream := openStream(t, w.srv, fresh)
			defer closeStream()
			if status != http.StatusOK {
				t.Fatalf("the new token's stream = %d, want 200", status)
			}
		})
	}
}

// inviteWithTask invites worker into alpha again with task and returns the new token.
func inviteWithTask(t *testing.T, w mailWorld, task string) string {
	t.Helper()
	var inv struct{ Code string }
	callTool(t, w.srv, w.lead, "bus_invite_member", `{"space":"alpha","name":"worker","task":"`+task+`"}`, &inv)
	status, body := do(t, w.srv, http.MethodPost, "/v1/invites/accept", inv.Code, "")
	if status != http.StatusOK {
		t.Fatalf("accept: %d %s", status, body)
	}
	return tokenOf(t, body)
}

func TestStreamOfARotatedAdminTokenEnds(t *testing.T) {
	t.Parallel()
	w := newMailWorld(t)
	callTool(t, w.srv, adminToken, "bus_join_space", `{"space":"alpha"}`, nil)
	old := streamUntilEnd(t, w.srv, adminToken)
	next := "rotated-admin-token-0123456789-abcdefgh"
	if status, body := do(t, w.srv, http.MethodPost, "/v1/admin/admin-token", adminToken, `{"token":"`+next+`"}`); status != http.StatusOK {
		t.Fatalf("rotating: %d %s", status, body)
	}
	callTool(t, w.srv, w.lead, "bus_send_direct", `{"space":"alpha","to":"admin","body":"secret after rotation"}`, nil)
	endsWithout(t, old, "secret")
}

func TestWaitOfARevokedMemberFails(t *testing.T) {
	t.Parallel()
	w := newMailWorld(t)
	done := make(chan int, 1)
	go func() { done <- waitStatus(w.srv, w.worker) }()
	time.Sleep(300 * time.Millisecond) // let the wait block
	callTool(t, w.srv, w.lead, "bus_revoke_member", `{"space":"alpha","name":"worker"}`, nil)
	invite(t, w.srv, w.lead, "alpha", "worker")
	callTool(t, w.srv, w.lead, "bus_send_direct", `{"space":"alpha","to":"worker","body":"wait-leak"}`, nil)
	select {
	case status := <-done:
		if status != http.StatusUnauthorized {
			t.Fatalf("the revoked wait = %d, want 401", status)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the revoked wait is still pending")
	}
}

// waitStatus runs a 15 s bus_wait as token and returns its status (0 when the call failed).
func waitStatus(srv *httptest.Server, token string) int {
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/tools/bus_wait", strings.NewReader(`{"timeoutSeconds":15}`))
	if err != nil {
		return 0
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := srv.Client().Do(req)
	if err != nil {
		return 0
	}
	defer func() { _ = res.Body.Close() }()
	return res.StatusCode
}

// tokenOf decodes the token of an invite acceptance body.
func tokenOf(t *testing.T, body []byte) string {
	t.Helper()
	var mem struct{ Token string }
	if err := json.Unmarshal(body, &mem); err != nil || mem.Token == "" {
		t.Fatalf("accept body %s: %v", body, err)
	}
	return mem.Token
}
