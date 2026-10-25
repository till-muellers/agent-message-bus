// Command tooldoc prints the Markdown tool ledger; run `go run ./cmd/tooldoc > TOOLS.md`.
package main

import (
	"fmt"
	"os"

	"github.com/till-muellers/agent-message-bus/server/internal/tools"
)

func main() {
	if _, err := fmt.Fprint(os.Stdout, tools.RenderMarkdown(tools.New(tools.Deps{}))); err != nil {
		fmt.Fprintf(os.Stderr, "tooldoc: writing ledger: %v\n", err)
		os.Exit(1)
	}
}
