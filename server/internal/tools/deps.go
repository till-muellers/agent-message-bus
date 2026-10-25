package tools

import (
	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/blackboards"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/messaging"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// Deps are the feature services the tools call into.
type Deps struct {
	Auth        *auth.Service
	Spaces      *spaces.Service
	Messaging   *messaging.Service
	Blackboards *blackboards.Service
	Inbox       *inbox.Service
}

// New builds the registry of every bus tool.
func New(d Deps) *Registry {
	var ts []Tool
	ts = append(ts, adminTools(d.Auth)...)
	ts = append(ts, spaceTools(d.Spaces)...)
	ts = append(ts, topicTools(d.Messaging)...)
	ts = append(ts, blackboardTools(d.Blackboards)...)
	ts = append(ts, inboxTools(d.Inbox)...)
	return NewRegistry(ts)
}
