package tools

import (
	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/blackboards"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/invites"
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
	Invites     *invites.Service
}

// New builds the registry of every bus tool. Services are only used inside
// Call, so zero-value Deps suffice to list or document the tools.
func New(d Deps) *Registry {
	var ts []Tool
	ts = append(ts, inGroup(GroupAdmin, adminTools(d.Auth, d.Spaces))...)
	ts = append(ts, inGroup(GroupSpaces, append(spaceTools(d.Spaces), inviteTools(d.Invites)...))...)
	ts = append(ts, inGroup(GroupMessaging, topicTools(d.Messaging))...)
	ts = append(ts, inGroup(GroupBlackboards, blackboardTools(d.Blackboards))...)
	ts = append(ts, inGroup(GroupInbox, inboxTools(d.Inbox))...)
	return NewRegistry(ts)
}
