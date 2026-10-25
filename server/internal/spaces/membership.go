package spaces

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
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
