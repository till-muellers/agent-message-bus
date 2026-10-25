package tools

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
)

//go:embed ledger_head.md.tmpl
var ledgerHead string

//go:embed ledger_tail.md.tmpl
var ledgerTail string

// groupOrder lists the tool groups in ledger order.
func groupOrder() []string {
	return []string{GroupAdmin, GroupSpaces, GroupMessaging, GroupBlackboards, GroupInbox}
}

// RenderMarkdown renders reg as the deterministic Markdown tool ledger (TOOLS.md).
func RenderMarkdown(reg *Registry) string {
	ts := reg.List()
	groups := groupTools(ts)
	var b strings.Builder
	b.WriteString(strings.ReplaceAll(ledgerHead, "{{count}}", strconv.Itoa(len(ts))))
	writeIndex(&b, groups)
	for _, g := range groupOrder() {
		if len(groups[g]) == 0 {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n", g)
		for _, tl := range groups[g] {
			writeToolSection(&b, tl)
		}
	}
	b.WriteString(ledgerTail)
	return b.String()
}

// groupTools buckets ts by group, keeping the registry's name order.
func groupTools(ts []Tool) map[string][]Tool {
	groups := map[string][]Tool{}
	for _, tl := range ts {
		groups[tl.Group] = append(groups[tl.Group], tl)
	}
	return groups
}

func writeIndex(b *strings.Builder, groups map[string][]Tool) {
	b.WriteString("## Index\n\n| Tool | Group | Access | Summary |\n| :--- | :--- | :--- | :--- |\n")
	for _, g := range groupOrder() {
		for _, tl := range groups[g] {
			access, summary := splitDescription(tl.Description)
			fmt.Fprintf(b, "| [%s](#%s) | [%s](#%s) | %s | %s |\n",
				tl.Name, anchor(tl.Name), g, anchor(g), cell(access), cell(summary))
		}
	}
	b.WriteString("\n")
}

func writeToolSection(b *strings.Builder, tl Tool) {
	fmt.Fprintf(b, "### %s\n\n%s\n\n", tl.Name, tl.Description)
	b.WriteString(argTable(tl.Schema))
	fmt.Fprintf(b, "\n**Result:** `%s`\n\n", tl.Result)
	fmt.Fprintf(b, "```sh\ncurl -X POST $BUS/v1/tools/%s -H \"Authorization: Bearer $TOKEN\" -d '%s'\n```\n\n",
		tl.Name, exampleArgs(tl.Schema))
}

// splitDescription splits desc into its first sentence (the access rule) and the rest.
func splitDescription(desc string) (access, summary string) {
	i := strings.Index(desc, ". ")
	if i < 0 {
		return desc, ""
	}
	return desc[:i+1], desc[i+2:]
}

// anchor returns the GitHub heading anchor for heading text.
func anchor(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

// cell escapes s for use inside a Markdown table cell.
func cell(s string) string { return strings.ReplaceAll(s, "|", `\|`) }
