// Package tools defines the single tool registry served over MCP and the RPC web API.
package tools

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

// Tool groups, in ledger order.
const (
	GroupAdmin       = "Admin"
	GroupSpaces      = "Spaces & membership"
	GroupMessaging   = "Direct messages & topics"
	GroupBlackboards = "Blackboards"
	GroupInbox       = "Inbox"
)

// Tool is one callable operation: its Meta and the call that decodes and runs its arguments.
type Tool struct {
	Meta
	Call func(ctx context.Context, p auth.Principal, args json.RawMessage) (any, error)
}

// inGroup sets group on every tool in ts and returns ts.
func inGroup(group string, ts []Tool) []Tool {
	for i := range ts {
		ts[i].Group = group
	}
	return ts
}

// Registry is an immutable, name-sorted set of tools; transports run them through Call.
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

// List returns the metadata of all tools sorted by name.
func (r *Registry) List() []Meta {
	metas := make([]Meta, len(r.list))
	for i, t := range r.list {
		metas[i] = t.Meta
	}
	return metas
}

// Call runs tool name with the JSON arguments args as p. An unknown name is not_found, a
// bad argument invalid; every transport maps these kinds like any other tool error.
func (r *Registry) Call(ctx context.Context, p auth.Principal, name string, args json.RawMessage) (any, error) {
	t, found := r.byName[name]
	if !found {
		return nil, apperr.New(apperr.NotFound, "tool %s not found", name)
	}
	return t.Call(ctx, p, args)
}
