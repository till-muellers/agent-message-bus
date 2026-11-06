package server_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// promptly is how long an idle stream or wait of a dead credential may outlive its death;
// the heartbeat recheck (20 s) must not be what ends it.
const promptly = 2 * time.Second

// deadCredential is one way to end a credential: kill runs as the revoking caller, token
// is the credential that dies and agent its agent.
type deadCredential struct {
	agent string
	token func(t *testing.T, w mailWorld) string
	kill  func(t *testing.T, w mailWorld)
}

func deadCredentials() map[string]deadCredential {
	worker := func(_ *testing.T, w mailWorld) string { return w.worker }
	return map[string]deadCredential{
		"member revoked": {"worker", worker, func(t *testing.T, w mailWorld) {
			callTool(t, w.srv, w.lead, "bus_revoke_member", `{"space":"alpha","name":"worker"}`, nil)
		}},
		"member left": {"worker", worker, func(t *testing.T, w mailWorld) {
			callTool(t, w.srv, w.worker, "bus_leave_space", `{"space":"alpha"}`, nil)
		}},
		"space deleted": {"worker", worker, func(t *testing.T, w mailWorld) {
			callTool(t, w.srv, w.lead, "bus_delete_space", `{"space":"alpha"}`, nil)
		}},
		"admin revoked": {"ops", func(t *testing.T, w mailWorld) string { return joinedAdmin(t, w, "ops") },
			func(t *testing.T, w mailWorld) {
				callTool(t, w.srv, adminToken, "bus_revoke_admin", `{"name":"ops"}`, nil)
			}},
		"admin token rotated": {"admin", func(t *testing.T, w mailWorld) string {
			callTool(t, w.srv, adminToken, "bus_join_space", `{"space":"alpha"}`, nil)
			return adminToken
		}, rotateAdminToken},
	}
}

func TestIdleStreamOfADeadTokenEndsPromptly(t *testing.T) {
	t.Parallel()
	for name, tc := range deadCredentials() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newMailWorld(t)
			lines := streamUntilEnd(t, w.srv, tc.token(t, w))
			untilConnected(t, w, tc.agent)
			tc.kill(t, w)
			deadline := time.After(promptly)
			for {
				select {
				case line, ok := <-lines:
					if ok && strings.Contains(line, "secret") {
						t.Fatalf("got %q", line)
					}
					if !ok {
						return
					}
				case <-deadline:
					t.Fatalf("the idle stream outlived its token by more than %s", promptly)
				}
			}
		})
	}
}

func TestIdleWaitOfADeadTokenFailsPromptly(t *testing.T) {
	t.Parallel()
	for name, tc := range deadCredentials() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newMailWorld(t)
			token := tc.token(t, w)
			emptyMailbox(t, w.srv, token)
			done := make(chan int, 1)
			go func() { done <- waitStatus(w.srv, token) }()
			untilConnected(t, w, tc.agent)
			tc.kill(t, w)
			select {
			case status := <-done:
				if status != http.StatusUnauthorized {
					t.Fatalf("the dead token's wait = %d, want 401", status)
				}
			case <-time.After(promptly):
				t.Fatalf("the idle wait outlived its token by more than %s", promptly)
			}
		})
	}
}

func TestReinvitedTokenStreamsRightAfterTheRevocationOfAnIdleStream(t *testing.T) {
	t.Parallel()
	w := newMailWorld(t)
	streamUntilEnd(t, w.srv, w.worker)
	untilConnected(t, w, "worker")
	callTool(t, w.srv, w.lead, "bus_revoke_member", `{"space":"alpha","name":"worker"}`, nil)
	fresh := invite(t, w.srv, w.lead, "alpha", "worker")
	deadline := time.Now().Add(promptly)
	for {
		status, _, closeStream := openStream(t, w.srv, fresh)
		closeStream()
		if status == http.StatusOK {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the re-invited token's stream = %d after %s, want 200", status, promptly)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
