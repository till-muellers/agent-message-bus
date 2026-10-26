package server_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
)

// readsWorld is the gui-access world plus leader "lead2" with spaces "other" and "quiet",
// an open invite for "pending" in demo (code pendingCode) and one for "later" in other.
type readsWorld struct {
	*webWorld
	lead2, pendingCode string
}

func newReadsWorld(t *testing.T) readsWorld {
	t.Helper()
	w := readsWorld{webWorld: newWebWorld(t, false)}
	var lead2, inv struct{ Token, Code string }
	callTool(t, w.api, adminToken, "bus_mint_leader", `{"name":"lead2"}`, &lead2)
	w.lead2 = lead2.Token
	callTool(t, w.api, w.lead2, "bus_create_space", `{"space":"other"}`, nil)
	callTool(t, w.api, w.lead2, "bus_create_space", `{"space":"quiet"}`, nil)
	callTool(t, w.api, w.lead, "bus_invite_member", `{"space":"demo","name":"pending"}`, &inv)
	w.pendingCode = inv.Code
	callTool(t, w.api, w.lead2, "bus_invite_member", `{"space":"other","name":"later"}`, nil)
	return w
}

// get signs in with token and returns the status and body of GET path.
func (w readsWorld) get(t *testing.T, token, path string) (int, string) {
	t.Helper()
	c, _ := w.signIn(t, token)
	return w.status(t, http.MethodGet, path, c)
}

type stateRow struct {
	Name            string
	Leaders, Admins []string
	Members         int
	Joined          bool
	Invites         *int
}

type stateList struct {
	You    struct{ Kind, Name string }
	Spaces []stateRow
	Inbox  struct{ Waiting int }
}

func TestStateList(t *testing.T) {
	t.Parallel()
	w := newReadsWorld(t)
	callTool(t, w.api, w.worker, "bus_send_direct", `{"space":"demo","to":"lead","body":"need a decision"}`, nil)
	demo := stateRow{Name: "demo", Leaders: []string{"lead"}, Admins: []string{}, Members: 1}
	other := stateRow{Name: "other", Leaders: []string{"lead2"}, Admins: []string{}}
	quiet := stateRow{Name: "quiet", Leaders: []string{"lead2"}, Admins: []string{}}
	with := func(r stateRow, invites *int, joined bool) stateRow { r.Invites, r.Joined = invites, joined; return r }
	tests := []struct {
		name, token, kind, you string
		spaces                 []stateRow
		waiting                int
	}{
		{"bootstrap admin sees all spaces with invite counts", adminToken, "admin", "admin",
			[]stateRow{with(demo, new(1), false), with(other, new(1), false), with(quiet, new(0), false)}, 0},
		{"minted admin sees all spaces with invite counts", w.ops, "admin", "ops",
			[]stateRow{with(demo, new(1), false), with(other, new(1), false), with(quiet, new(0), false)}, 0},
		{"leader sees all spaces, invite counts only where it leads", w.lead, "leader", "lead",
			[]stateRow{with(demo, new(1), true), with(other, nil, false), with(quiet, nil, false)}, 1},
		{"member sees its own space without invite counts", w.worker, "member", "worker",
			[]stateRow{with(demo, nil, true)}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, body := w.get(t, tc.token, "/web/api/state")
			var got stateList
			if err := json.Unmarshal([]byte(body), &got); status != http.StatusOK || err != nil {
				t.Fatalf("GET /web/api/state = %d %s (%v)", status, body, err)
			}
			if got.You.Kind != tc.kind || got.You.Name != tc.you || got.Inbox.Waiting != tc.waiting {
				t.Fatalf("you/inbox = %+v %+v, want %s %s waiting %d", got.You, got.Inbox, tc.kind, tc.you, tc.waiting)
			}
			if !reflect.DeepEqual(got.Spaces, tc.spaces) {
				t.Fatalf("spaces = %s, want %+v", body, tc.spaces)
			}
		})
	}
	if status, body := w.status(t, http.MethodGet, "/web/api/state", nil); status != http.StatusUnauthorized {
		t.Fatalf("no session: %d %s", status, body)
	}
}

type pendingInvite struct{ Name, InvitedBy, ExpiresAt string }

type stateSpace struct {
	You      struct{ Kind, Name string }
	Overview map[string]json.RawMessage
	Invites  *[]pendingInvite
	Inbox    struct{ Waiting int }
}

func TestStateSpace(t *testing.T) {
	t.Parallel()
	w := newReadsWorld(t)
	callTool(t, w.api, w.lead, "bus_send_topic", `{"space":"demo","topic":"all","body":"topic-secret"}`, nil)
	callTool(t, w.api, w.worker, "bus_create_blackboard", `{"space":"demo","blackboard":"b"}`, nil)
	callTool(t, w.api, w.worker, "bus_create_thread", `{"space":"demo","blackboard":"b","title":"plan","body":"thread-secret"}`, nil)
	pending := []pendingInvite{{Name: "pending", InvitedBy: "lead"}}
	tests := []struct {
		name, token, space string
		invites            *[]pendingInvite
	}{
		{"admin gets the open invites", adminToken, "demo", &pending},
		{"leader of the space gets the open invites", w.lead, "demo", &pending},
		{"member gets no invites key", w.worker, "demo", nil},
		{"leader with no open invite gets an empty list", w.lead2, "quiet", &[]pendingInvite{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, body := w.get(t, tc.token, "/web/api/state?space="+tc.space)
			var got stateSpace
			if err := json.Unmarshal([]byte(body), &got); status != http.StatusOK || err != nil {
				t.Fatalf("GET state %s = %d %s (%v)", tc.space, status, body, err)
			}
			for _, key := range []string{"space", "you", "participants", "topics", "blackboards", "recentThreads"} {
				if _, ok := got.Overview[key]; !ok {
					t.Fatalf("overview lacks %q: %s", key, body)
				}
			}
			for _, secret := range []string{"topic-secret", "thread-secret", w.pendingCode} {
				if strings.Contains(body, secret) {
					t.Fatalf("state carries %q: %s", secret, body)
				}
			}
			if strings.Contains(body, `"invites"`) != (tc.invites != nil) {
				t.Fatalf("invites key presence: %s", body)
			}
			expectInvites(t, got.Invites, tc.invites)
		})
	}
	denied := []struct {
		name, token, space string
		status             int
		msg                string
	}{
		{"unknown space", adminToken, "ghost", http.StatusNotFound, "space ghost not found"},
		{"leader not in the space", w.lead2, "demo", http.StatusForbidden, "not a participant of space demo"},
	}
	for _, tc := range denied {
		if status, body := w.get(t, tc.token, "/web/api/state?space="+tc.space); status != tc.status || !strings.Contains(body, tc.msg) {
			t.Fatalf("%s: %d %s, want %d %q", tc.name, status, body, tc.status, tc.msg)
		}
	}
	if status, body := w.status(t, http.MethodGet, "/web/api/state?space=demo", nil); status != http.StatusUnauthorized {
		t.Fatalf("no session: %d %s", status, body)
	}
}

// expectInvites compares invites without their expiry, which must be set.
func expectInvites(t *testing.T, got, want *[]pendingInvite) {
	t.Helper()
	if want == nil {
		return
	}
	if got == nil || len(*got) != len(*want) {
		t.Fatalf("invites = %v, want %v", got, *want)
	}
	for i, inv := range *got {
		if inv.ExpiresAt == "" {
			t.Fatalf("invite %d has no expiry", i)
		}
		inv.ExpiresAt = ""
		if inv != (*want)[i] {
			t.Fatalf("invite %d = %+v, want %+v", i, inv, (*want)[i])
		}
	}
}

func TestWebThreadReads(t *testing.T) {
	t.Parallel()
	w := newReadsWorld(t)
	callTool(t, w.api, w.worker, "bus_create_blackboard", `{"space":"demo","blackboard":"b"}`, nil)
	var th struct{ ThreadID int64 }
	callTool(t, w.api, w.worker, "bus_create_thread", `{"space":"demo","blackboard":"b","title":"plan","body":"first"}`, &th)
	callTool(t, w.api, w.lead2, "bus_create_blackboard", `{"space":"other","blackboard":"b"}`, nil)
	var foreign struct{ ThreadID int64 }
	callTool(t, w.api, w.lead2, "bus_create_thread", `{"space":"other","blackboard":"b","title":"x","body":"y"}`, &foreign)
	tests := []struct {
		name, path string
		status     int
		want       string
	}{
		{"threads of a board", "/web/api/spaces/demo/blackboards/b/threads", http.StatusOK, `"title":"plan"`},
		{"unknown board", "/web/api/spaces/demo/blackboards/ghost/threads", http.StatusNotFound, "not_found"},
		{"a thread with its posts", "/web/api/spaces/demo/threads/" + jsonInt(th.ThreadID), http.StatusOK, `"body":"first"`},
		{"a thread of another space", "/web/api/spaces/demo/threads/" + jsonInt(foreign.ThreadID), http.StatusNotFound, "not_found"},
		{"a malformed thread id", "/web/api/spaces/demo/threads/abc", http.StatusBadRequest, "threadId must be an integer"},
	}
	c, _ := w.signIn(t, adminToken)
	for _, tc := range tests {
		if status, body := w.status(t, http.MethodGet, tc.path, c); status != tc.status || !strings.Contains(body, tc.want) {
			t.Fatalf("%s: %d %s, want %d %q", tc.name, status, body, tc.status, tc.want)
		}
	}
	if status, body := w.status(t, http.MethodGet, "/web/api/spaces/demo/threads/"+jsonInt(th.ThreadID), nil); status != http.StatusUnauthorized {
		t.Fatalf("no session: %d %s", status, body)
	}
}

// inboxOf returns the entries GET /web/api/inbox lists for cookie c.
func (w readsWorld) inboxOf(t *testing.T, c *http.Cookie, query string) []inbox.Entry {
	t.Helper()
	status, body := w.status(t, http.MethodGet, "/web/api/inbox"+query, c)
	var es []inbox.Entry
	if err := json.Unmarshal([]byte(body), &es); status != http.StatusOK || err != nil || es == nil {
		t.Fatalf("GET /web/api/inbox%s = %d %s (%v)", query, status, body, err)
	}
	return es
}

func bodies(es []inbox.Entry) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.Body)
	}
	return out
}

func TestWebInboxReadOnly(t *testing.T) {
	t.Parallel()
	w := newReadsWorld(t)
	callTool(t, w.api, w.ops, "bus_join_space", `{"space":"demo"}`, nil)
	emptyMailbox(t, w.api, w.lead)
	callTool(t, w.api, w.worker, "bus_send_direct", `{"space":"demo","to":"lead","body":"need a decision"}`, nil)
	callTool(t, w.api, w.worker, "bus_send_direct", `{"space":"demo","to":"ops","body":"for ops"}`, nil)
	t.Run("own scope only", func(t *testing.T) {
		for token, want := range map[string][]string{w.lead: {"need a decision"}, w.ops: {"for ops"}, adminToken: {}, w.worker: {}} {
			c, _ := w.signIn(t, token)
			if got := bodies(w.inboxOf(t, c, "")); !reflect.DeepEqual(got, want) {
				t.Fatalf("inbox = %v, want %v", got, want)
			}
		}
	})
	lead, _ := w.signIn(t, w.lead)
	es := w.inboxOf(t, lead, "?limit=500")
	t.Run("after and limit", func(t *testing.T) {
		if got := w.inboxOf(t, lead, "?after="+jsonInt(es[0].ID)); len(got) != 0 {
			t.Fatalf("after the last entry = %v", got)
		}
		for query, msg := range map[string]string{"?limit=501": "limit must be 1..500", "?limit=x": "limit must be an integer",
			"?after=x": "after must be an integer"} {
			if status, body := w.status(t, http.MethodGet, "/web/api/inbox"+query, lead); status != http.StatusBadRequest || !strings.Contains(body, msg) {
				t.Fatalf("%s: %d %s, want 400 %q", query, status, body, msg)
			}
		}
	})
	t.Run("405 on mutating methods", func(t *testing.T) {
		for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			if status, body := w.status(t, m, "/web/api/inbox", lead); status != http.StatusMethodNotAllowed {
				t.Fatalf("%s /web/api/inbox = %d %s", m, status, body)
			}
		}
	})
	t.Run("no notice, no ack", func(t *testing.T) {
		if reply := rpc(t, w.api, w.lead, "bus_whoami", `{}`); reply.Mail == nil {
			t.Fatalf("the web read marked the edge noticed: %+v", reply)
		}
		var after []inbox.Entry
		callTool(t, w.api, w.lead, "bus_inbox_read", `{}`, &after)
		if got := bodies(after); !reflect.DeepEqual(got, []string{"need a decision"}) {
			t.Fatalf("inbox after the web reads = %v", got)
		}
	})
	t.Run("no stream slot", func(t *testing.T) {
		status, _, closeStream := openStream(t, w.api, w.lead)
		defer closeStream()
		if status != http.StatusOK {
			t.Fatalf("lead's stream while the web reads: %d", status)
		}
		w.inboxOf(t, lead, "")
	})
	if status, body := w.status(t, http.MethodGet, "/web/api/inbox", nil); status != http.StatusUnauthorized {
		t.Fatalf("no session: %d %s", status, body)
	}
}
