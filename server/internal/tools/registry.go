// Package tools defines the single tool registry served over MCP and the RPC web API.
package tools

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
)

// Tool groups, in ledger order.
const (
	GroupAdmin       = "Admin"
	GroupSpaces      = "Spaces & membership"
	GroupMessaging   = "Direct messages & topics"
	GroupBlackboards = "Blackboards"
	GroupInbox       = "Inbox"
)

// Tool is one callable operation with a JSON Schema for its arguments.
type Tool struct {
	Name        string
	Description string
	// Result is a one-line sketch of the success result shape.
	Result string
	// Group is the ledger section the tool belongs to (one of the Group* constants).
	Group  string
	Schema map[string]any
	Call   func(ctx context.Context, p auth.Principal, args json.RawMessage) (any, error)
}

// inGroup sets group on every tool in ts and returns ts.
func inGroup(group string, ts []Tool) []Tool {
	for i := range ts {
		ts[i].Group = group
	}
	return ts
}

// Registry is an immutable, name-sorted set of tools.
type Registry struct {
	list   []Tool
	byName map[string]Tool
}

// NewRegistry builds a registry from ts, sorted by name.
func NewRegistry(ts []Tool) *Registry {
	list := append([]Tool(nil), ts...)
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	byName := make(map[string]Tool, len(list))
	for _, t := range list {
		byName[t.Name] = t
	}
	return &Registry{list: list, byName: byName}
}

// List returns all tools sorted by name.
func (r *Registry) List() []Tool { return append([]Tool(nil), r.list...) }

// Get returns the tool named name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.byName[name]
	return t, ok
}
