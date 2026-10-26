// Package guide holds the agent guide: how leaders, members and admins work on the bus.
// The same text is served as MCP instructions, as GET /v1/guide and in TOOLS.md.
package guide

import _ "embed"

// Text is the guide as Markdown, identical for every caller and at most 4000 characters.
//
//go:embed guide.md
var Text string
