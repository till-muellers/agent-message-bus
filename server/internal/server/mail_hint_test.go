package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
)

const hintAlpha = "[agent-bus] You have mail: 1 waiting in alpha. " +
	"Call bus_inbox_read, handle the entries, then bus_inbox_ack {upTo}."

// mailWorld: leader "lead" with space "alpha" and members "worker" and "peer", all mailboxes empty.
type mailWorld struct {
	srv                *httptest.Server
	lead, worker, peer string
}

func newMailWorld(t *testing.T) mailWorld {
	t.Helper()
	srv := newServer(t)
	var lead struct{ Token string }
	callTool(t, srv, adminToken, "bus_mint_leader", `{"name":"lead"}`, &lead)
	callTool(t, srv, lead.Token, "bus_create_space", `{"space":"alpha"}`, nil)
	w := mailWorld{srv: srv, lead: lead.Token,
		worker: invite(t, srv, lead.Token, "alpha", "worker"), peer: invite(t, srv, lead.Token, "alpha", "peer")}
	for _, tok := range []string{w.lead, w.worker, w.peer} {
		emptyMailbox(t, srv, tok)
	}
	return w
}

// invite invites member name into space as leader and returns the accepted member token.
func invite(t *testing.T, srv *httptest.Server, leader, space, name string) string {
	t.Helper()
	var inv struct{ Code string }
	callTool(t, srv, leader, "bus_invite_member", `{"space":"`+space+`","name":"`+name+`"}`, &inv)
	status, body := do(t, srv, http.MethodPost, "/v1/invites/accept", inv.Code, "")
	var mem struct{ Token string }
	if err := json.Unmarshal(body, &mem); status != http.StatusOK || err != nil {
		t.Fatalf("accept %s: status %d body %s err %v", name, status, body, err)
	}
	return mem.Token
}

// emptyMailbox acknowledges everything token sees and swallows a pending hint.
func emptyMailbox(t *testing.T, srv *httptest.Server, token string) {
	t.Helper()
	var es []inbox.Entry
	callTool(t, srv, token, "bus_inbox_read", `{"limit":500}`, &es)
	if len(es) > 0 {
		callTool(t, srv, token, "bus_inbox_ack", `{"upTo":`+jsonInt(es[len(es)-1].ID)+`}`, nil)
	}
	rpc(t, srv, token, "bus_whoami", `{}`)
}

func jsonInt(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (w mailWorld) send(t *testing.T, from, to string) {
	t.Helper()
	callTool(t, w.srv, from, "bus_send_direct", `{"space":"alpha","to":"`+to+`","body":"hi"}`, nil)
}

// rpcReply is a POST /v1/tools success body; Mail is nil when the key is absent.
type rpcReply struct {
	Result json.RawMessage
	Mail   *inbox.Notice
	Keys   []string
}

func rpc(t *testing.T, srv *httptest.Server, token, tool, args string) rpcReply {
	t.Helper()
	status, body := do(t, srv, http.MethodPost, "/v1/tools/"+tool, token, args)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); status != http.StatusOK || err != nil {
		t.Fatalf("%s: status %d body %s err %v", tool, status, body, err)
	}
	r := rpcReply{Result: raw["result"]}
	for k := range raw {
		r.Keys = append(r.Keys, k)
	}
	if m, ok := raw["mail"]; ok {
		r.Mail = &inbox.Notice{}
		if err := json.Unmarshal(m, r.Mail); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

// mcpCall calls tool over MCP as token and returns its content texts and structured content.
func mcpCall(t *testing.T, srv *httptest.Server, token, tool string, args map[string]any) ([]string, any) {
	t.Helper()
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	s, err := c.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: bearer{token}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	res, err := s.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil || res.IsError {
		t.Fatalf("%s: err %v result %+v", tool, err, res)
	}
	var texts []string
	for _, c := range res.Content {
		texts = append(texts, c.(*mcp.TextContent).Text)
	}
	return texts, res.StructuredContent
}

// hintOf returns the MCP hint (the trailing text after the tool's own text), or "".
func hintOf(texts []string) string {
	if len(texts) < 2 {
		return ""
	}
	return texts[len(texts)-1]
}

// transports call bus_whoami as token and return the hint they carried ("" for none).
var transports = map[string]func(t *testing.T, srv *httptest.Server, token string) string{
	"rpc": func(t *testing.T, srv *httptest.Server, token string) string {
		r := rpc(t, srv, token, "bus_whoami", `{}`)
		if r.Mail == nil {
			return ""
		}
		want := inbox.Notice{Waiting: 1, Critical: 0, Spaces: []string{"alpha"}}
		if !reflect.DeepEqual(*r.Mail, want) {
			t.Fatalf("mail = %+v, want %+v", *r.Mail, want)
		}
		return hintAlpha
	},
	"mcp": func(t *testing.T, srv *httptest.Server, token string) string {
		texts, _ := mcpCall(t, srv, token, "bus_whoami", nil)
		return hintOf(texts)
	},
}

func TestHintOnce(t *testing.T) {
	t.Parallel()
	for name, whoami := range transports {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newMailWorld(t)
			w.send(t, w.peer, "worker")
			if got := whoami(t, w.srv, w.worker); got != hintAlpha {
				t.Fatalf("first call hint = %q, want %q", got, hintAlpha)
			}
			if got := whoami(t, w.srv, w.worker); got != "" {
				t.Fatalf("second call hint = %q", got)
			}
			w.send(t, w.peer, "worker")
			if got := whoami(t, w.srv, w.worker); got != "" {
				t.Fatalf("hint after more mail before ack = %q", got)
			}
			emptyMailbox(t, w.srv, w.worker)
			w.send(t, w.peer, "worker")
			if got := whoami(t, w.srv, w.worker); got != hintAlpha {
				t.Fatalf("hint after a full ack = %q, want %q", got, hintAlpha)
			}
		})
	}
}

func TestHintKeepsStructuredContent(t *testing.T) {
	t.Parallel()
	w := newMailWorld(t)
	w.send(t, w.peer, "worker")
	texts, structured := mcpCall(t, w.srv, w.worker, "bus_whoami", nil)
	if len(texts) != 2 || texts[1] != hintAlpha {
		t.Fatalf("contents = %q", texts)
	}
	var plain any
	if err := json.Unmarshal([]byte(texts[0]), &plain); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(structured, plain) {
		t.Fatalf("structured %v != plain %v", structured, plain)
	}
}

func TestLookingClearsHint(t *testing.T) {
	t.Parallel()
	for _, look := range []string{"bus_inbox_read", "bus_wait"} {
		t.Run(look, func(t *testing.T) {
			t.Parallel()
			w := newMailWorld(t)
			w.send(t, w.peer, "worker")
			if r := rpc(t, w.srv, w.worker, look, `{}`); r.Mail != nil {
				t.Fatalf("%s carried mail %+v", look, *r.Mail)
			}
			if r := rpc(t, w.srv, w.worker, "bus_whoami", `{}`); r.Mail != nil {
				t.Fatalf("whoami after %s carried mail %+v", look, *r.Mail)
			}
		})
	}
}

func TestAckNeverHintsItsOwnEntries(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		readFirst bool
	}{
		{"entry got from bus_inbox_read", true},
		{"unnoticed edge acknowledged unseen", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := newMailWorld(t)
			w.send(t, w.peer, "worker")
			upTo := int64(1) << 40
			if c.readFirst {
				var es []inbox.Entry
				callTool(t, w.srv, w.worker, "bus_inbox_read", `{}`, &es)
				upTo = es[0].ID
			}
			r := rpc(t, w.srv, w.worker, "bus_inbox_ack", `{"upTo":`+jsonInt(upTo)+`}`)
			if string(r.Result) != `{"acked":1}` || r.Mail != nil {
				t.Fatalf("ack = %s mail %+v", r.Result, r.Mail)
			}
		})
	}
}

func TestRPCMailFieldAbsent(t *testing.T) {
	t.Parallel()
	w := newMailWorld(t)
	r := rpc(t, w.srv, w.worker, "bus_whoami", `{}`)
	if !reflect.DeepEqual(r.Keys, []string{"result"}) {
		t.Fatalf("body keys = %v, want only result", r.Keys)
	}
	w.send(t, w.peer, "worker")
	r = rpc(t, w.srv, w.worker, "bus_whoami", `{}`)
	if len(r.Keys) != 2 || r.Mail == nil {
		t.Fatalf("body keys = %v, want result and mail", r.Keys)
	}
}

func TestHintScopes(t *testing.T) {
	t.Parallel()
	const tail = " Call bus_inbox_read, handle the entries, then bus_inbox_ack {upTo}."
	t.Run("leader lists every space", func(t *testing.T) {
		t.Parallel()
		w := newMailWorld(t)
		callTool(t, w.srv, w.lead, "bus_create_space", `{"space":"beta"}`, nil)
		beta := invite(t, w.srv, w.lead, "beta", "other")
		emptyMailbox(t, w.srv, w.lead)
		w.send(t, w.worker, "lead")
		callTool(t, w.srv, beta, "bus_send_direct", `{"space":"beta","to":"lead","body":"hi"}`, nil)
		texts, _ := mcpCall(t, w.srv, w.lead, "bus_whoami", nil)
		if want := "[agent-bus] You have mail: 2 waiting in alpha, beta." + tail; hintOf(texts) != want {
			t.Fatalf("hint = %q, want %q", hintOf(texts), want)
		}
	})
	t.Run("admin gets its own mailbox", func(t *testing.T) {
		t.Parallel()
		w := newMailWorld(t)
		var ops struct{ Token string }
		callTool(t, w.srv, adminToken, "bus_mint_admin", `{"name":"ops"}`, &ops)
		callTool(t, w.srv, ops.Token, "bus_join_space", `{"space":"alpha"}`, nil)
		emptyMailbox(t, w.srv, ops.Token)
		w.send(t, w.worker, "ops")
		texts, _ := mcpCall(t, w.srv, ops.Token, "bus_whoami", nil)
		if want := "[agent-bus] You have mail: 1 waiting in alpha." + tail; hintOf(texts) != want {
			t.Fatalf("hint = %q, want %q", hintOf(texts), want)
		}
	})
	t.Run("critical count", func(t *testing.T) {
		t.Parallel()
		w := newMailWorld(t)
		callTool(t, w.srv, w.lead, "bus_send_direct", `{"space":"alpha","to":"worker","body":"now","critical":true}`, nil)
		texts, _ := mcpCall(t, w.srv, w.worker, "bus_whoami", nil)
		if want := "[agent-bus] You have mail: 1 waiting in alpha (1 critical)." + tail; hintOf(texts) != want {
			t.Fatalf("hint = %q, want %q", hintOf(texts), want)
		}
	})
}
