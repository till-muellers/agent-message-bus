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
}

func TestEveryToolStatesAccess(t *testing.T) {
	t.Parallel()
	ts := New(Deps{}).List()
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

func TestPitfalls(t *testing.T) {
	t.Parallel()
	tests := []struct{ tool, pitfall string }{
		{"bus_inbox_read", "omp sessions receive entries automatically and need not poll"},
		{"bus_inbox_read", "it never blocks and reading does not acknowledge"},
		{"bus_inbox_ack", "it deletes every entry with id up to upTo, so never acknowledge past what was handled"},
		{"bus_create_topic", "nobody else is notified, so subscribers must be told the topic name"},
		{"bus_create_blackboard", "a member's boards are deleted when it leaves or is revoked"},
		{"bus_leave_space", "a member's boards, inbox entries, subscriptions and token are deleted"},
		{"bus_send_direct", "the recipient must be a participant of the same space"},
		{"bus_send_direct", "an agent cannot send a direct message to itself"},
		{"bus_invite_member", "the code is single-use and expires after 10 minutes; the user redeems it with " +
			"/bus accept <url> <code> or omp --bus-url <url> --bus-accept <code>, other clients with POST /v1/invites/accept"},
		{"bus_invite_member", "inviting the same name again gives a new code and an additional token"},
		{"bus_join_space", "a space can have several leaders"},
		{"bus_subscribe_topic", "the all topic cannot be subscribed or unsubscribed, because everyone receives it"},
		{"bus_close_topic", "the all topic cannot be closed, and a closed topic's name can be reused"},
		{"bus_create_thread", "the creator is subscribed and gets later posts by others"},
		{"bus_revoke_member", "an admin may also revoke leaders, the last leader included, and the space is deleted when nobody remains"},
		{"bus_delete_space", "an admin may delete any space, even one with other leaders"},
	}
	for _, tc := range tests {
		t.Run(tc.tool, func(t *testing.T) {
			t.Parallel()
			if d := toolNamed(t, tc.tool).Description; !strings.Contains(d, tc.pitfall) {
				t.Fatalf("description %q lacks pitfall %q", d, tc.pitfall)
			}
		})
	}
}

func TestCriticalProperty(t *testing.T) {
	t.Parallel()
	const want = "Leaders and admins only. omp recipients are interrupted (steer); other clients see critical: true."
	for _, name := range []string{"bus_send_direct", "bus_send_topic"} {
		props := toolNamed(t, name).Schema["properties"].(map[string]any)
		if got := props["critical"].(map[string]any)["description"]; got != want {
			t.Fatalf("%s critical = %q, want %q", name, got, want)
		}
	}
}

func TestLedgerIndexUsesAccess(t *testing.T) {
	t.Parallel()
	doc := RenderMarkdown(New(Deps{}))
	for _, want := range []string{
		"| [bus_whoami](#bus_whoami) | [Spaces & membership](#spaces--membership) | Any token; no side effects. | " +
			"Returns the caller's kind, name and the spaces it participates in. |\n",
		"| [bus_add_post](#bus_add_post) | [Blackboards](#blackboards) | Participants and admins. | " +
			"Appends a post and notifies the thread's subscribers except the author. |\n",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("index lacks row %q", want)
		}
	}
}

func TestLedgerDocumentsSpawnWorker(t *testing.T) {
	t.Parallel()
	doc := RenderMarkdown(New(Deps{}))
	for _, want := range []string{
		"### bus_spawn_worker\n\nStarts a worker agent",
		"Leaders of the space and admins.",
		"| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |",
		"| `name` | string | yes |",
		"| `task` | string | no |",
		"| `cwd` | string | no |",
		"**Result:** `{name, space, launcher}`",
		"each worker needs its own name",
		"a name already used by a leader agent fails with conflict, and without a terminal launcher the leader invites by hand",
		"the task is delivered to the worker as a direct message from the caller (a leader or an admin)",
		"| `task` | string | no | Task for the worker, delivered as a direct message from you, 1..65536 bytes: goal, done criteria and where to report. |",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("ledger lacks %q", want)
		}
	}
}
