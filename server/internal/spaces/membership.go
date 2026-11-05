package spaces

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// Role is a participant's role within one space.
type Role string

const (
	RoleAdmin  Role = "admin" // participant row of a joined admin
	RoleLeader Role = "leader"
	RoleMember Role = "member"
)

// Membership is a resolved (principal, space) pair.
type Membership struct {
	SpaceID   int64
	SpaceName string
	AgentID   int64
	AgentName string
	Role      Role // participant role; "" when an admin has not joined
	Admin     bool // caller is an admin principal (authority without participation)
}

// Leads reports whether m has leader authority in the space; every authority check uses it.
func (m Membership) Leads() bool { return m.Admin || m.Role == RoleLeader }

// Joined reports whether m holds a participant row in the space.
func (m Membership) Joined() bool { return m.Role != "" }

// MaySeeInvites reports whether m may list, count and cancel the space's pending invites:
// leader rights, as Leads. It is the one rule the web GUI and invites ask.
func (m Membership) MaySeeInvites() bool { return m.Leads() }

// RequireJoined fails with Forbidden unless m holds a participant row; only participants
// receive messages, so subscribing needs joining.
func (m Membership) RequireJoined() error {
	if !m.Joined() {
		return apperr.New(apperr.Forbidden, "join space %s first; only participants receive messages", m.SpaceName)
	}
	return nil
}

// DepartureHook cleans feature state when a participant leaves or is revoked.
// It runs inside the departure transaction; returned entries are inserted there
// and published after commit.
type DepartureHook interface {
	OnDeparture(ctx context.Context, tx db.DBTX, m Membership) ([]inbox.Entry, error)
}

// CreationHook seeds feature state when a space is created; m is the creator's
// leader membership. It runs inside the creating transaction.
type CreationHook interface {
	OnSpaceCreated(ctx context.Context, tx db.DBTX, m Membership) error
}

// RevocationHook removes feature state held for name in a space that does not depend on
// a participant row, such as pending invite codes. It runs inside the revoking transaction
// and reports whether it removed anything.
type RevocationHook interface {
	OnRevoke(ctx context.Context, tx db.DBTX, spaceID int64, name string) (bool, error)
}

// Hooks are the feature callbacks a Service runs inside its transactions.
type Hooks struct {
	Departure  []DepartureHook
	Creation   []CreationHook
	Revocation []RevocationHook
}

// Notify copies proto once per recipient, sent by m in its space.
func (m Membership) Notify(recipients []int64, proto inbox.Entry) []inbox.Entry {
	return inbox.Sender{SpaceID: m.SpaceID, Space: m.SpaceName, Name: m.AgentName}.Notify(recipients, proto)
}
