package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// contract records the GUI backend's real responses as fixtures (file name → body).
type contract struct {
	*webWorld
	t   *testing.T
	out map[string][]byte
}

// TestWebContractFixtures marshals the responses of the real /web handlers into
// server/internal/web/testdata, which gui/tests/api/contract.test.ts decodes with the GUI's
// production decoders (contracts.md §4.1, §10.1–§10.4, §10.6). Times, secrets and invite ids
// are normalized; regenerate with -update.
func TestWebContractFixtures(t *testing.T) {
	t.Parallel()
	c := &contract{webWorld: contractWorld(t), t: t, out: map[string][]byte{}}
	c.recordReads()
	c.recordMutations()
	c.recordAdmins()
	checkFixtures(t, c.out)
}

// contractWorld is the mutation world (blackboard "b", thread 1 "plan", topic "notes")
// with mail for lead and worker, an open invite for "new" and an extra space "scratch".
func contractWorld(t *testing.T) *webWorld {
	t.Helper()
	w := mutationWorld(t)
	emptyMailbox(t, w.api, w.lead)
	callTool(t, w.api, w.worker, "bus_send_direct", `{"space":"demo","to":"lead","body":"need a decision"}`, nil)
	callTool(t, w.api, w.lead, "bus_add_post", `{"space":"demo","threadId":1,"body":"reply"}`, nil)
	callTool(t, w.api, w.lead, "bus_send_direct", `{"space":"demo","to":"worker","body":"stop","critical":true}`, nil)
	callTool(t, w.api, w.lead, "bus_send_topic", `{"space":"demo","topic":"all","body":"heads up"}`, nil)
	callTool(t, w.api, w.lead, "bus_invite_member", `{"space":"demo","name":"new"}`, nil)
	callTool(t, w.api, w.lead, "bus_create_space", `{"space":"scratch"}`, nil)
	return w
}

// record sends method path with body as s and stores the normalized 200 body as name.json.
func (c *contract) record(name, method, path, body string, s session) {
	c.t.Helper()
	status, raw, _ := c.send(c.t, method, path, body, s, s.csrf)
	if status != http.StatusOK {
		c.t.Fatalf("%s %s = %d %s", method, path, status, raw)
	}
	c.out[name+".json"] = normalized(c.t, raw)
}

func (c *contract) recordReads() {
	c.t.Helper()
	res := c.webDo(c.t, http.MethodPost, "/web/session", "Bearer "+c.worker, nil)
	body := readBody(c.t, res)
	if res.StatusCode != http.StatusOK {
		c.t.Fatalf("sign in: %d %s", res.StatusCode, body)
	}
	c.out["session.json"] = normalized(c.t, body)
	lead, worker := c.open(c.t, c.lead), c.open(c.t, c.worker)
	c.record("state-list", http.MethodGet, "/web/api/state", "", lead)
	c.record("state-space-leader", http.MethodGet, "/web/api/state?space=demo", "", lead)
	c.record("state-space-member", http.MethodGet, "/web/api/state?space=demo", "", worker)
	c.record("threads", http.MethodGet, "/web/api/spaces/demo/blackboards/b/threads", "", worker)
	c.record("thread", http.MethodGet, "/web/api/spaces/demo/threads/1", "", worker)
	c.record("inbox", http.MethodGet, "/web/api/inbox?limit=500", "", worker)
	c.record("sign-out", http.MethodDelete, "/web/session", "", worker)
}

func (c *contract) recordMutations() {
	c.t.Helper()
	lead := c.open(c.t, c.lead)
	c.record("invite", http.MethodPost, "/web/api/spaces/demo/invites", `{"name":"later","task":"start"}`, lead)
	c.record("cancel-invite", http.MethodDelete, "/web/api/spaces/demo/invites/"+c.firstInvite(lead), "", lead)
	c.record("post", http.MethodPost, "/web/api/spaces/demo/threads/1/posts", `{"body":"more"}`, lead)
	c.record("send", http.MethodPost, "/web/api/spaces/demo/topics/notes/messages", `{"body":"hi","critical":false}`, lead)
	c.record("revoke", http.MethodPost, "/web/api/spaces/demo/revoke", `{"name":"worker","confirm":"demo/worker"}`, lead)
	c.record("delete-space", http.MethodDelete, "/web/api/spaces/scratch", `{"confirm":"scratch"}`, lead)
}

// firstInvite returns the id of demo's open invite that expires first, as s lists it.
func (c *contract) firstInvite(s session) string {
	c.t.Helper()
	status, raw, _ := c.send(c.t, http.MethodGet, "/web/api/spaces/demo/invites", "", s, "")
	var open []struct{ ID string }
	if err := json.Unmarshal([]byte(raw), &open); status != http.StatusOK || err != nil || len(open) == 0 {
		c.t.Fatalf("open invites: %d %s %v", status, raw, err)
	}
	return open[0].ID
}

func (c *contract) recordAdmins() {
	c.t.Helper()
	ops := c.open(c.t, c.ops)
	c.record("admins", http.MethodGet, "/web/api/admins", "", ops)
	c.record("mint-admin", http.MethodPost, "/web/api/admins", `{"name":"ops2"}`, ops)
	c.record("revoke-admin", http.MethodPost, "/web/api/admins/ops2/revoke", "", ops)
}
