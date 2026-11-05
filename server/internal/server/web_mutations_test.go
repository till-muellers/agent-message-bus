package server_test

import (
	"bytes"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
)

// mutationWorld seeds the gui-mutations.feature background on top of webWorld: member
// "worker" created blackboard "b" with thread 1 "plan" in space "demo", and leader "lead"
// created topic "notes" there.
func mutationWorld(t *testing.T) *webWorld {
	t.Helper()
	w := newWebWorld(t, false)
	callTool(t, w.api, w.worker, "bus_create_blackboard", `{"space":"demo","blackboard":"b"}`, nil)
	callTool(t, w.api, w.worker, "bus_create_thread", `{"space":"demo","blackboard":"b","title":"plan","body":"first"}`, nil)
	callTool(t, w.api, w.lead, "bus_create_topic", `{"space":"demo","topic":"notes"}`, nil)
	emptyMailbox(t, w.api, w.worker)
	return w
}

// mintLeader mints leader name and returns its token.
func (w *webWorld) mintLeader(t *testing.T, name string) string {
	t.Helper()
	var out struct{ Token string }
	callTool(t, w.api, adminToken, "bus_mint_leader", `{"name":"`+name+`"}`, &out)
	return out.Token
}

// spaceExists reports whether space is listed for the bootstrap admin.
func (w *webWorld) spaceExists(t *testing.T, space string) bool {
	t.Helper()
	var rows []struct{ Name string }
	callTool(t, w.api, adminToken, "bus_list_spaces", `{}`, &rows)
	for _, r := range rows {
		if r.Name == space {
			return true
		}
	}
	return false
}

// participants returns name → role of space.
func (w *webWorld) participants(t *testing.T, space string) map[string]string {
	t.Helper()
	var rows []struct{ Name, Role string }
	callTool(t, w.api, adminToken, "bus_list_participants", `{"space":"`+space+`"}`, &rows)
	out := map[string]string{}
	for _, r := range rows {
		out[r.Name] = r.Role
	}
	return out
}

// pendingInvites counts the open invites of space demo as the GUI state shows them.
func (w *webWorld) pendingInvites(t *testing.T) int {
	t.Helper()
	var state struct{ Invites []json.RawMessage }
	status, body, _ := w.send(t, http.MethodGet, "/web/api/state?space=demo", "", w.open(t, adminToken), "")
	if err := json.Unmarshal([]byte(body), &state); status != http.StatusOK || err != nil {
		t.Fatalf("state: %d %s %v", status, body, err)
	}
	return len(state.Invites)
}

// inboxOf returns token's waiting inbox entries.
func (w *webWorld) inboxOf(t *testing.T, token string) []inbox.Entry {
	t.Helper()
	var es []inbox.Entry
	callTool(t, w.api, token, "bus_inbox_read", `{"limit":500}`, &es)
	return es
}

// posts returns the posts of thread 1 in space demo.
func (w *webWorld) posts(t *testing.T) []struct{ Author, Body string } {
	t.Helper()
	var th struct {
		Posts []struct{ Author, Body string }
	}
	callTool(t, w.api, adminToken, "bus_read_thread", `{"space":"demo","threadId":1}`, &th)
	return th.Posts
}

// mutationCase is one GUI mutation by the session of as ("" = no session); setup runs
// before the request, check inspects the outcome.
type mutationCase struct {
	name, as, method, path, body string
	status                       int
	want                         string
	setup                        func(t *testing.T, w *webWorld)
	check                        func(t *testing.T, w *webWorld, body string)
}

func TestWebMutations(t *testing.T) {
	t.Parallel()
	var cases []mutationCase
	for _, part := range [][]mutationCase{inviteCases(), revokeCases(), lastLeaderCases(), postCases(),
		sendCases(), deleteCases()} {
		cases = append(cases, part...)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runMutation(t, tc)
		})
	}
}

func runMutation(t *testing.T, tc mutationCase) {
	t.Helper()
	w := mutationWorld(t)
	if tc.setup != nil {
		tc.setup(t, w)
	}
	tokens := map[string]string{"ops": w.ops, "worker": w.worker}
	s := w.open(t, tokens[tc.as])
	status, body, h := w.send(t, tc.method, tc.path, tc.body, s, s.csrf)
	if status != tc.status || !strings.Contains(body, tc.want) {
		t.Fatalf("%s %s = %d %s, want %d %q", tc.method, tc.path, status, body, tc.status, tc.want)
	}
	if cc := h.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cc)
	}
	if tc.check != nil {
		tc.check(t, w, body)
	}
}

const invitesPath = "/web/api/spaces/demo/invites"

func inviteCases() []mutationCase {
	return []mutationCase{
		{name: "invite shows the code once and it is accepted", as: "ops", method: http.MethodPost, path: invitesPath,
			body: `{"name":"new","task":"start"}`, status: http.StatusOK, want: `"code":"`, check: checkInvited},
		{name: "invite in a space the admin is not in", as: "ops", method: http.MethodPost,
			path: "/web/api/spaces/other/invites", body: `{"name":"new"}`, status: http.StatusOK,
			want: `"space":"other"`, setup: otherSpace},
		{name: "invalid member name", as: "ops", method: http.MethodPost, path: invitesPath, body: `{"name":"Bad Name"}`,
			status: http.StatusBadRequest, want: "name must match ^[a-z0-9][a-z0-9_-]{0,62}$", check: noInvite},
		{name: "an admin name cannot be invited", as: "ops", method: http.MethodPost, path: invitesPath,
			body: `{"name":"ops"}`, status: http.StatusConflict, want: "agent ops is an admin agent", check: noInvite},
		{name: "member session cannot invite", as: "worker", method: http.MethodPost, path: invitesPath,
			body: `{"name":"new"}`, status: http.StatusForbidden, want: `"code":"forbidden"`, check: noInvite},
		{name: "invite without a session", method: http.MethodPost, path: invitesPath, body: `{"name":"new"}`,
			status: http.StatusUnauthorized, want: "session required", check: noInvite},
		{name: "an unknown body field is rejected like a tool argument", as: "ops", method: http.MethodPost,
			path: invitesPath, body: `{"name":"new","bogus":1}`, status: http.StatusBadRequest,
			want: `invalid arguments: json: unknown field \"bogus\"`, check: noInvite},
	}
}

// otherSpace makes leader "lead2" create space "other".
func otherSpace(t *testing.T, w *webWorld) {
	t.Helper()
	callTool(t, w.api, w.mintLeader(t, "lead2"), "bus_create_space", `{"space":"other"}`, nil)
}

func checkInvited(t *testing.T, w *webWorld, body string) {
	t.Helper()
	var inv struct{ Name, Space, Code string }
	if err := json.Unmarshal([]byte(body), &inv); err != nil || inv.Name != "new" || inv.Space != "demo" {
		t.Fatalf("invite body %s: %v", body, err)
	}
	if strings.Contains(w.stateBody(t), inv.Code) {
		t.Fatal("the reloaded space shows the invite code")
	}
	if status, out := do(t, w.api, http.MethodPost, "/v1/invites/accept", inv.Code, ""); status != http.StatusOK {
		t.Fatalf("accept: %d %s", status, out)
	}
}

// stateBody is the GUI state of space demo for a fresh admin session.
func (w *webWorld) stateBody(t *testing.T) string {
	t.Helper()
	status, body, _ := w.send(t, http.MethodGet, "/web/api/state?space=demo", "", w.open(t, adminToken), "")
	if status != http.StatusOK || !strings.Contains(body, `"name":"new"`) {
		t.Fatalf("state lists no open invite for new: %d %s", status, body)
	}
	return body
}

func noInvite(t *testing.T, w *webWorld, _ string) {
	t.Helper()
	if n := w.pendingInvites(t); n != 0 {
		t.Fatalf("pending invites = %d, want 0", n)
	}
}

const revokePath = "/web/api/spaces/demo/revoke"

func revokeCases() []mutationCase {
	return []mutationCase{
		{name: "confirmed revoke removes the member", as: "ops", method: http.MethodPost, path: revokePath,
			body: `{"name":"worker","confirm":"demo/worker"}`, status: http.StatusOK, want: `"deleted":true`,
			check: workerGone},
		{name: "revoking an absent member reports nothing removed", as: "ops", method: http.MethodPost, path: revokePath,
			body: `{"name":"ghost","confirm":"demo/ghost"}`, status: http.StatusOK, want: `"deleted":false`},
		{name: "revoke without confirmation", as: "ops", method: http.MethodPost, path: revokePath,
			body: `{"name":"worker"}`, status: http.StatusBadRequest,
			want: `{"error":{"code":"invalid","message":"confirm the revoke of worker in space demo"}}`, check: workerStays},
		{name: "revoke with a wrong confirmation", as: "ops", method: http.MethodPost, path: revokePath,
			body: `{"name":"worker","confirm":"demo/lead"}`, status: http.StatusBadRequest,
			want: "confirm the revoke of worker in space demo", check: workerStays},
		{name: "revoke a member of a space the admin is not in", as: "ops", method: http.MethodPost,
			path: "/web/api/spaces/other/revoke", body: `{"name":"helper","confirm":"other/helper"}`,
			status: http.StatusOK, want: `"deleted":true`, setup: helperInOther, check: helperGone},
		{name: "revoke without a session", method: http.MethodPost, path: revokePath,
			body: `{"name":"worker","confirm":"demo/worker"}`, status: http.StatusUnauthorized,
			want: "session required", check: workerStays},
	}
}

func workerGone(t *testing.T, w *webWorld, _ string) {
	t.Helper()
	if w.authenticates(t, w.worker) {
		t.Fatal("revoked worker still authenticates")
	}
}

func workerStays(t *testing.T, w *webWorld, _ string) {
	t.Helper()
	if w.participants(t, "demo")["worker"] != "member" {
		t.Fatal("worker is no longer a member of demo")
	}
}

// helperInOther makes leader "lead" create space "other" and invite member "helper".
func helperInOther(t *testing.T, w *webWorld) {
	t.Helper()
	callTool(t, w.api, w.lead, "bus_create_space", `{"space":"other"}`, nil)
	w.helper = invite(t, w.api, w.lead, "other", "helper")
}

func helperGone(t *testing.T, w *webWorld, _ string) {
	t.Helper()
	if w.authenticates(t, w.helper) {
		t.Fatal("revoked helper still authenticates")
	}
}

func lastLeaderCases() []mutationCase {
	return []mutationCase{
		{name: "revoke a leader of a space", as: "ops", method: http.MethodPost, path: revokePath,
			body: `{"name":"lead2","confirm":"demo/lead2"}`, status: http.StatusOK, want: `"deleted":true`,
			setup: lead2Joins, check: participantsAre(map[string]string{"lead": "leader", "worker": "member"})},
		{name: "revoke the last leader keeps the space", as: "ops", method: http.MethodPost, path: revokePath,
			body: `{"name":"lead","confirm":"demo/lead"}`, status: http.StatusOK, want: `"deleted":true`,
			check: lastLeaderGone},
		{name: "revoke the last participant deletes the space", as: "ops", method: http.MethodPost,
			path: "/web/api/spaces/solo/revoke", body: `{"name":"lead2","confirm":"solo/lead2"}`, status: http.StatusOK,
			want: `"deleted":true`, setup: soloSpace, check: spaceGone("solo")},
	}
}

func lead2Joins(t *testing.T, w *webWorld) {
	t.Helper()
	callTool(t, w.api, w.mintLeader(t, "lead2"), "bus_join_space", `{"space":"demo"}`, nil)
}

func soloSpace(t *testing.T, w *webWorld) {
	t.Helper()
	callTool(t, w.api, w.mintLeader(t, "lead2"), "bus_create_space", `{"space":"solo"}`, nil)
}

func participantsAre(want map[string]string) func(t *testing.T, w *webWorld, body string) {
	return func(t *testing.T, w *webWorld, _ string) {
		t.Helper()
		got := w.participants(t, "demo")
		if len(got) != len(want) {
			t.Fatalf("participants = %v, want %v", got, want)
		}
		for name, role := range want {
			if got[name] != role {
				t.Fatalf("participants = %v, want %v", got, want)
			}
		}
	}
}

func lastLeaderGone(t *testing.T, w *webWorld, body string) {
	t.Helper()
	participantsAre(map[string]string{"worker": "member"})(t, w, body)
	if !w.spaceExists(t, "demo") || !w.authenticates(t, w.lead) {
		t.Fatal("space demo gone or lead's token no longer authenticates")
	}
}

func spaceGone(space string) func(t *testing.T, w *webWorld, body string) {
	return func(t *testing.T, w *webWorld, _ string) {
		t.Helper()
		if w.spaceExists(t, space) {
			t.Fatalf("space %s still exists", space)
		}
	}
}

const postsPath = "/web/api/spaces/demo/threads/1/posts"

func postCases() []mutationCase {
	return []mutationCase{
		{name: "post to a thread", as: "ops", method: http.MethodPost, path: postsPath, body: `{"body":"hello"}`,
			status: http.StatusOK, want: `"postId":`, check: checkPosted},
		{name: "empty post body", as: "ops", method: http.MethodPost, path: postsPath, body: `{"body":""}`,
			status: http.StatusBadRequest, want: "body must be 1..65536 bytes", check: onePost},
		{name: "thread id must be an integer", as: "ops", method: http.MethodPost,
			path: "/web/api/spaces/demo/threads/x/posts", body: `{"body":"hello"}`, status: http.StatusBadRequest,
			want: "threadId must be an integer", check: onePost},
		{name: "post without a session", method: http.MethodPost, path: postsPath, body: `{"body":"hello"}`,
			status: http.StatusUnauthorized, want: "session required", check: onePost},
	}
}

func checkPosted(t *testing.T, w *webWorld, _ string) {
	t.Helper()
	ps := w.posts(t)
	if len(ps) != 2 || ps[1].Author != "ops" || ps[1].Body != "hello" {
		t.Fatalf("posts = %+v", ps)
	}
	es := w.inboxOf(t, w.worker)
	if len(es) != 1 || es[0].Type != "thread_post" || es[0].From != "ops" || es[0].ThreadID != 1 {
		t.Fatalf("worker inbox = %+v", es)
	}
}

func onePost(t *testing.T, w *webWorld, _ string) {
	t.Helper()
	if ps := w.posts(t); len(ps) != 1 {
		t.Fatalf("posts = %+v, want one", ps)
	}
}

const notesPath = "/web/api/spaces/demo/topics/notes/messages"

func sendCases() []mutationCase {
	return []mutationCase{
		{name: "send to a topic", as: "ops", method: http.MethodPost, path: notesPath, body: `{"body":"hi"}`,
			status: http.StatusOK, want: `"ok":true`, setup: workerSubscribes, check: received("notes", "hi", false)},
		{name: "send to all", as: "ops", method: http.MethodPost, path: "/web/api/spaces/demo/topics/all/messages",
			body: `{"body":"hi"}`, status: http.StatusOK, want: `"ok":true`, check: received("all", "hi", false)},
		{name: "send a critical message", as: "ops", method: http.MethodPost, path: notesPath,
			body: `{"body":"now","critical":true}`, status: http.StatusOK, want: `"ok":true`, setup: workerSubscribes,
			check: received("notes", "now", true)},
		{name: "member session cannot send critical", as: "worker", method: http.MethodPost, path: notesPath,
			body: `{"body":"now","critical":true}`, status: http.StatusForbidden,
			want: "only leaders may send critical messages"},
		{name: "send without a session", method: http.MethodPost, path: notesPath, body: `{"body":"hi"}`,
			status: http.StatusUnauthorized, want: "session required", setup: workerSubscribes, check: nothingReceived},
	}
}

func workerSubscribes(t *testing.T, w *webWorld) {
	t.Helper()
	callTool(t, w.api, w.worker, "bus_subscribe_topic", `{"space":"demo","topic":"notes"}`, nil)
}

func received(topic, body string, critical bool) func(t *testing.T, w *webWorld, _ string) {
	return func(t *testing.T, w *webWorld, _ string) {
		t.Helper()
		es := w.inboxOf(t, w.worker)
		if len(es) != 1 || es[0].Type != "topic_message" || es[0].From != "ops" || es[0].Topic != topic ||
			es[0].Body != body || es[0].Critical != critical {
			t.Fatalf("worker inbox = %+v", es)
		}
	}
}

func nothingReceived(t *testing.T, w *webWorld, _ string) {
	t.Helper()
	if es := w.inboxOf(t, w.worker); len(es) != 0 {
		t.Fatalf("worker inbox = %+v, want empty", es)
	}
}

const spacePath = "/web/api/spaces/demo"

func deleteCases() []mutationCase {
	return []mutationCase{
		{name: "confirmed delete", as: "ops", method: http.MethodDelete, path: spacePath, body: `{"confirm":"demo"}`,
			status: http.StatusOK, want: `"ok":true`, check: deleted},
		{name: "delete a space with other leaders", as: "ops", method: http.MethodDelete, path: spacePath,
			body: `{"confirm":"demo"}`, status: http.StatusOK, want: `"ok":true`, setup: lead2Joins, check: deleted},
		{name: "delete without confirmation", as: "ops", method: http.MethodDelete, path: spacePath,
			status: http.StatusBadRequest, check: demoStays,
			want: `{"error":{"code":"invalid","message":"confirm the deletion of space demo"}}`},
		{name: "delete with a wrong confirmation", as: "ops", method: http.MethodDelete, path: spacePath,
			body: `{"confirm":"other"}`, status: http.StatusBadRequest, want: "confirm the deletion of space demo",
			check: demoStays},
		{name: "delete without a session", method: http.MethodDelete, path: spacePath, body: `{"confirm":"demo"}`,
			status: http.StatusUnauthorized, want: "session required", check: demoStays},
	}
}

func deleted(t *testing.T, w *webWorld, body string) {
	t.Helper()
	spaceGone("demo")(t, w, body)
	workerGone(t, w, body)
}

func demoStays(t *testing.T, w *webWorld, _ string) {
	t.Helper()
	if !w.spaceExists(t, "demo") {
		t.Fatal("space demo was deleted")
	}
}

// csrfRoutes are the W3 routes with a body that would succeed and a check that nothing happened.
var csrfRoutes = []struct {
	name, method, path, body string
	setup                    func(t *testing.T, w *webWorld)
	unchanged                func(t *testing.T, w *webWorld, body string)
}{
	{"invite", http.MethodPost, invitesPath, `{"name":"new"}`, nil, noInvite},
	{"revoke", http.MethodPost, revokePath, `{"name":"worker","confirm":"demo/worker"}`, nil, workerStays},
	{"post", http.MethodPost, postsPath, `{"body":"hello"}`, nil, onePost},
	{"send", http.MethodPost, notesPath, `{"body":"hi"}`, workerSubscribes, nothingReceived},
	{"delete", http.MethodDelete, spacePath, `{"confirm":"demo"}`, nil, demoStays},
}

func TestWebMutationsCSRF(t *testing.T) {
	t.Parallel()
	for _, rt := range csrfRoutes {
		for _, header := range []string{"missing", "foreign session"} {
			t.Run(rt.name+" "+header, func(t *testing.T) {
				t.Parallel()
				w := mutationWorld(t)
				if rt.setup != nil {
					rt.setup(t, w)
				}
				s := w.open(t, w.ops)
				csrf := map[string]string{"missing": "", "foreign session": w.open(t, w.ops).csrf}[header]
				status, body, _ := w.send(t, rt.method, rt.path, rt.body, s, csrf)
				if status != http.StatusForbidden || !strings.Contains(body, "invalid CSRF token") {
					t.Fatalf("%s %s = %d %s, want 403 invalid CSRF token", rt.method, rt.path, status, body)
				}
				rt.unchanged(t, w, body)
			})
		}
	}
}

// captureLog points slog.Default() (and the log package behind it) at a buffer until t
// ends. The services capture slog.Default() when the world is built, so build it after
// this call; a test using it must not run in parallel.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev, prevOut, prevFlags := slog.Default(), log.Writer(), log.Flags()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev); log.SetOutput(prevOut); log.SetFlags(prevFlags) })
	return &buf
}

// TestMutationLogLine swaps the default logger, so it must not run in parallel.
func TestMutationLogLine(t *testing.T) {
	buf := captureLog(t)
	w := mutationWorld(t)
	callTool(t, w.api, w.lead, "bus_create_space", `{"space":"other"}`, nil)
	s := w.open(t, w.ops)
	for _, rq := range []struct{ method, path, body string }{
		{http.MethodPost, invitesPath, `{"name":"new"}`},
		{http.MethodPost, postsPath, `{"body":"x"}`},
		{http.MethodPost, "/web/api/spaces/other/topics/all/messages", `{"body":"hi"}`},
		{http.MethodPost, "/web/api/spaces/other/threads/1/posts", `{"body":"x"}`},
		{http.MethodPost, revokePath, `{"name":"lead"}`},
		{http.MethodPost, revokePath, `{"name":"worker","confirm":"demo/worker"}`},
		{http.MethodDelete, "/web/api/spaces/other", `{"confirm":"other"}`},
	} {
		w.send(t, rq.method, rq.path, rq.body, s, s.csrf)
	}
	want := []string{"invite demo new", "post demo 1", "send other all", "revoke demo worker", "delete-space other other"}
	if got := mutationLines(t, buf, w.ops); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("web mutation lines = %q, want %q", got, want)
	}
}

// mutationLines returns "action space target" of every "web mutation" line by actor ops,
// failing on any line that is not INFO or that carries the token.
func mutationLines(t *testing.T, buf *bytes.Buffer, token string) []string {
	t.Helper()
	var got []string
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var line map[string]any
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("log line %q: %v", raw, err)
		}
		if line["msg"] != "web mutation" {
			continue
		}
		if line["level"] != "INFO" || line["actor"] != "ops" || strings.Contains(raw, token) {
			t.Fatalf("web mutation line %s", raw)
		}
		got = append(got, strings.Join([]string{str(line["action"]), str(line["space"]), str(line["target"])}, " "))
	}
	return got
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
