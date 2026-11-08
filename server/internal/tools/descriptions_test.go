package tools

import (
	"strings"
	"testing"
)

// wantAccess is the current access sentence of every tool (contracts.md §3.1).
var wantAccess = map[string]string{
	"bus_mint_leader":        "Admins only.",
	"bus_mint_admin":         "Admins only.",
	"bus_list_admins":        "Admins only; no side effects.",
	"bus_revoke_admin":       "Admins only.",
	"bus_whoami":             "Any token; no side effects.",
	"bus_list_spaces":        "Leaders and admins see all spaces, members only their own; no side effects.",
	"bus_list_participants":  "Participants and admins; no side effects.",
	"bus_space_overview":     "Participants and admins; no side effects.",
	"bus_list_topics":        "Participants and admins; no side effects.",
	"bus_list_blackboards":   "Participants and admins; no side effects.",
	"bus_list_threads":       "Participants and admins; no side effects.",
	"bus_read_thread":        "Participants and admins; no side effects.",
	"bus_create_space":       "Leaders and admins.",
	"bus_join_space":         "Leaders and admins.",
	"bus_leave_space":        "Participants.",
	"bus_delete_space":       "Leaders of the space once no other leader remains, and admins.",
	"bus_invite_member":      "Leaders of the space and admins.",
	"bus_revoke_member":      "Leaders of the space and admins.",
	"bus_send_direct":        "Participants and admins; critical requires a leader or an admin.",
	"bus_send_topic":         "Participants and admins; only leaders and admins may send to all or send critical.",
	"bus_create_topic":       "Participants and admins.",
	"bus_create_blackboard":  "Participants and admins.",
	"bus_create_thread":      "Participants and admins.",
	"bus_add_post":           "Participants and admins.",
	"bus_subscribe_topic":    "Participants (admins join first).",
	"bus_unsubscribe_topic":  "Participants (admins join first).",
	"bus_subscribe_thread":   "Participants (admins join first).",
	"bus_unsubscribe_thread": "Participants (admins join first).",
	"bus_close_topic":        "The topic's creator, leaders of the space and admins.",
	"bus_delete_blackboard":  "The board's owner, leaders of the space and admins.",
	"bus_remove_thread":      "The thread's creator, leaders of the space and admins.",
	"bus_inbox_read":         "Leaders, members and admins.",
	"bus_inbox_ack":          "Leaders, members and admins.",
	"bus_wait":               "Leaders, members and admins.",
}

func TestEveryToolStatesAccess(t *testing.T) {
	t.Parallel()
	ts := Metadata()
	if len(ts) != len(wantAccess) {
		t.Fatalf("registry has %d tools, access table %d", len(ts), len(wantAccess))
	}
	for _, tl := range ts {
		t.Run(tl.Name, func(t *testing.T) {
			t.Parallel()
			if tl.Access != wantAccess[tl.Name] {
				t.Fatalf("Access = %q, want %q", tl.Access, wantAccess[tl.Name])
			}
			if !strings.Contains(tl.Description, tl.Access) {
				t.Fatalf("Description %q lacks access %q", tl.Description, tl.Access)
			}
			for _, prefix := range []string{tl.Access, "Participants only", "Leaders only", "Any token"} {
				if strings.HasPrefix(tl.Description, prefix) {
					t.Fatalf("Description starts with access phrase %q", prefix)
				}
			}
		})
	}
}

func TestLedgerIndexUsesAccess(t *testing.T) {
	t.Parallel()
	doc := RenderMarkdown(Metadata())
	for _, tl := range Metadata() {
		row := "| [" + tl.Name + "](#" + tl.Name + ") | [" + tl.Group + "]"
		i := strings.Index(doc, row)
		if i < 0 {
			t.Fatalf("index lacks a row for %s", tl.Name)
		}
		line := doc[i : i+strings.Index(doc[i:], "\n")]
		if !strings.Contains(line, " | "+cell(tl.Access)+" | ") {
			t.Fatalf("index row %q lacks access %q", line, tl.Access)
		}
	}
}

func TestLedgerDocumentsSpawnWorker(t *testing.T) {
	t.Parallel()
	doc := RenderMarkdown(Metadata())
	for _, want := range []string{
		"### bus_spawn_worker\n\n",
		"| `space` | string | yes |",
		"| `name` | string | yes |",
		"| `task` | string | no |",
		"| `cwd` | string | no |",
		"**Result:** `{name, space, launcher}`",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("ledger lacks %q", want)
		}
	}
}
