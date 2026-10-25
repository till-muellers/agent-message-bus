package spaces

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// Role is a participant's role within one space.
type Role string

const (
	RoleLeader Role = "leader"
	RoleMember Role = "member"
)

// Membership is a resolved (principal, space) pair.
type Membership struct {
	SpaceID   int64
	SpaceName string
	AgentID   int64
	AgentName string
	Role      Role
}

// DepartureHook cleans feature state when a participant leaves or is revoked.
// It runs inside the departure transaction; returned entries are inserted there
// and published after commit.
type DepartureHook interface {
	OnDeparture(ctx context.Context, tx auth.DBTX, m Membership) ([]inbox.Entry, error)
}

// CreationHook seeds feature state when a space is created; m is the creator's
// leader membership. It runs inside the creating transaction.
type CreationHook interface {
	OnSpaceCreated(ctx context.Context, tx db.DBTX, m Membership) error
}

// Notify copies proto once per recipient, stamping recipient, space and sender m.
func (m Membership) Notify(recipients []int64, proto inbox.Entry) []inbox.Entry {
	es := make([]inbox.Entry, 0, len(recipients))
	for _, id := range recipients {
		e := proto
		e.AgentID, e.SpaceID, e.Space, e.From = id, m.SpaceID, m.SpaceName, m.AgentName
		es = append(es, e)
	}
	return es
}
