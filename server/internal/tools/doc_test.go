package tools

import (
	"os"
	"strings"
	"testing"
)

func TestToolsLedgerIsCurrent(t *testing.T) {
	want, err := os.ReadFile("../../TOOLS.md")
	if err != nil {
		t.Fatalf("reading TOOLS.md: %v", err)
	}
	if RenderMarkdown(New(Deps{})) != string(want) {
		t.Fatal("TOOLS.md is stale: run go run ./cmd/tooldoc > TOOLS.md")
	}
}

func toolNamed(t *testing.T, name string) Tool {
	t.Helper()
	tl, ok := New(Deps{}).Get(name)
	if !ok {
		t.Fatalf("no tool %s", name)
	}
	return tl
}

func TestArgumentTable(t *testing.T) {
	cases := []struct {
		tool string
		want string
	}{
		{"bus_add_post", "| Argument | Type | Required | Description |\n| :--- | :--- | :--- | :--- |\n" +
			"| `body` | string | yes | Message body, 1..65536 bytes. |\n" +
			"| `space` | string | yes | Space name, matching ^[a-z0-9][a-z0-9_-]{0,62}$. |\n" +
			"| `threadId` | integer | yes | Thread id. |\n"},
		{"bus_inbox_read", "| Argument | Type | Required | Description |\n| :--- | :--- | :--- | :--- |\n" +
			"| `after` | integer | no | Return entries with id greater than this; default 0. |\n" +
			"| `limit` | integer | no | Maximum entries, 1..500; default 100. |\n"},
		{"bus_whoami", "No arguments.\n"},
	}
	for _, c := range cases {
		t.Run(c.tool, func(t *testing.T) {
			if got := argTable(toolNamed(t, c.tool).Schema); got != c.want {
				t.Fatalf("got\n%s\nwant\n%s", got, c.want)
			}
		})
	}
}

func TestToolSectionAndIndexShareAnchor(t *testing.T) {
	doc := RenderMarkdown(New(Deps{}))
	for _, want := range []string{
		"\n### bus_add_post\n",
		"| [bus_add_post](#bus_add_post) | [Blackboards](#blackboards) | Participants and admins. |",
		"\n## Blackboards\n",
		`-d '{"body":"<body>","space":"<space>","threadId":1}'`,
		"**Result:** `{postId, receipts: [{to, delivery: live|queued}]}`",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("ledger lacks %q", want)
		}
	}
}

func TestAnchor(t *testing.T) {
	cases := map[string]string{
		"bus_add_post":             "bus_add_post",
		"Spaces & membership":      "spaces--membership",
		"Direct messages & topics": "direct-messages--topics",
	}
	for in, want := range cases {
		if got := anchor(in); got != want {
			t.Fatalf("anchor(%q) = %q, want %q", in, got, want)
		}
	}
}
