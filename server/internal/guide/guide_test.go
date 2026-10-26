package guide_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/till-muellers/agent-message-bus/server/internal/guide"
)

func TestGuideBudget(t *testing.T) {
	n := utf8.RuneCountInString(guide.Text)
	if n == 0 || n > 4000 {
		t.Fatalf("guide has %d characters, want 1..4000", n)
	}
}

// TestGuideCoversRoles pins one phrase per required statement (spec/guide.feature).
func TestGuideCoversRoles(t *testing.T) {
	cases := []struct{ topic, phrase string }{
		{"roles", "Leaders orchestrate; members do the work and report back."},
		{"first steps", "Call `bus_space_overview {space}`"},
		{"orders", "A direct message from a leader or an admin is an order. Your first one is your task."},
		{"mail in omp", "entries arrive by themselves as `agent-bus` messages"},
		{"acknowledged in omp", "They are acknowledged once you have seen them."},
		{"queued mail", "without stopping your work, or start a turn when you are idle"},
		{"critical", "`CRITICAL` entries come from a leader or an admin and interrupt you: handle them first."},
		{"mail elsewhere", "may end with `[agent-bus] You have mail: …`"},
		{"ack what you handled", "`bus_inbox_ack {upTo}` with the highest id you handled"},
		{"waiting for mail", "Call `bus_wait {timeoutSeconds}`"},
		{"member limits", "You cannot send to `all` or send critical messages."},
		{"when to report", "Report to the sender of your task with `bus_send_direct` when you are done, blocked, or need a decision. No progress chatter."},
		{"report fallback", "report to a leader of the space"},
		{"lasting results", "in a blackboard thread"},
		{"inbox vanishes", "An entry is deleted once acknowledged"},
		{"hand-off", "first post your state to a thread on a leader's board"},
		{"who ends membership", "Leave only when a leader tells you to."},
		{"leader revokes", "`bus_revoke_member`"},
		{"spawning", "Give each its own name and a task with the goal, the done criteria and where to report."},
		{"all and critical", "`all` is for announcements to everyone. `critical` only to stop or redirect someone now."},
		{"wind down", "The last leader cannot leave while others remain."},
		{"errors", "`unauthenticated`: your token is gone"},
		{"secrets", "Never post tokens or invite codes."},
		{"admins", "every leader right in every space without joining it (including inviting and spawning workers)"},
		{"admin superset", "delete any space and revoke members and leaders anywhere"},
		{"admin reachability", "join a space with `bus_join_space` to be reachable there; leaving keeps your authority"},
		{"admin tools", "Manage admins with `bus_list_admins` and `bus_revoke_admin`. Leave the work to leaders."},
		{"subagent tools", "(`bus_list_blackboards`, `bus_list_threads`, `bus_read_thread`, `bus_create_thread`, `bus_add_post`)"},
		{"subagent identity", "a first line `[via subagent <id>]`, and the main session is not notified of them"},
		{"subagent escalation", "`write agent://<id>`, then `wait`s"},
	}
	for _, c := range cases {
		t.Run(c.topic, func(t *testing.T) {
			if !strings.Contains(guide.Text, c.phrase) {
				t.Fatalf("guide lacks %q", c.phrase)
			}
		})
	}
}

func TestGuideErrorCodes(t *testing.T) {
	for _, code := range []string{"invalid", "forbidden", "not_found", "conflict", "unauthenticated"} {
		if !strings.Contains(guide.Text, "`"+code+"`: ") {
			t.Fatalf("guide does not say what to do about %s", code)
		}
	}
}

func TestGuideNeverSaysSkill(t *testing.T) {
	if strings.Contains(strings.ToLower(guide.Text), "skill") {
		t.Fatal(`guide contains "skill"`)
	}
}
