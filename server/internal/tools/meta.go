package tools

import "reflect"

// Meta describes a tool as agents and the ledger see it: name, texts and argument schema.
type Meta struct {
	Name string
	// Description is the full text served over MCP and GET /v1/tools: purpose first, then
	// the Access sentence and the pitfalls an agent cannot see from the schema.
	Description string
	// Access is the exact access sentence contained in Description; the ledger's Access column.
	Access string
	// Result is a one-line sketch of the success result shape.
	Result string
	// Group is the ledger section the tool belongs to (one of the Group* constants).
	Group  string
	Schema map[string]any
	// argsType is the struct the tool's call decodes the arguments into; it must match Schema.
	argsType reflect.Type
}

// Metadata returns the Meta of every bus tool, sorted by name, for documentation (the
// TOOLS.md ledger). It needs no services: the definitions bind them only inside each call,
// and the returned Meta cannot be called.
func Metadata() []Meta { return New(Deps{}).List() }
