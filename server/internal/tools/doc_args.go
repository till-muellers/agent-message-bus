package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// docArg is one schema property as shown in the ledger.
type docArg struct {
	name, kind, desc string
	required         bool
}

// docArgs lists schema's properties, required ones first, each part sorted by name.
func docArgs(schema map[string]any) []docArg {
	props, _ := schema["properties"].(map[string]any)
	required, _ := schema["required"].([]string)
	args := make([]docArg, 0, len(props))
	for name, raw := range props {
		p, _ := raw.(map[string]any)
		kind, _ := p["type"].(string)
		desc, _ := p["description"].(string)
		args = append(args, docArg{name: name, kind: kind, desc: desc, required: slices.Contains(required, name)})
	}
	sort.Slice(args, func(i, j int) bool {
		if args[i].required != args[j].required {
			return args[i].required
		}
		return args[i].name < args[j].name
	})
	return args
}

// argTable renders schema's properties as a Markdown table.
func argTable(schema map[string]any) string {
	args := docArgs(schema)
	if len(args) == 0 {
		return "No arguments.\n"
	}
	var b strings.Builder
	b.WriteString("| Argument | Type | Required | Description |\n| :--- | :--- | :--- | :--- |\n")
	for _, a := range args {
		req := "no"
		if a.required {
			req = "yes"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", a.name, a.kind, req, cell(a.desc))
	}
	return b.String()
}

// exampleArgs builds example JSON arguments from schema's required properties.
func exampleArgs(schema map[string]any) string {
	ex := map[string]any{}
	for _, a := range docArgs(schema) {
		if a.required {
			ex[a.name] = placeholder(a)
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(ex); err != nil {
		// Unreachable: ex holds only strings, ints and bools.
		panic(fmt.Sprintf("encoding example args: %v", err))
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// placeholder is an example value for a: "<name>" for strings, 1 or true otherwise.
func placeholder(a docArg) any {
	switch a.kind {
	case "integer":
		return 1
	case "boolean":
		return true
	default:
		return "<" + a.name + ">"
	}
}
