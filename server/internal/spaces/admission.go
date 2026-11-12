package spaces

import (
	"context"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// memberAccounts vets member names, mints the member tokens an Admitter hands out and
// revokes a re-admitted member's old tokens.
type memberAccounts interface {
	CheckMemberName(ctx context.Context, q db.DBTX, name string) error
	AdmitMember(ctx context.Context, tx db.DBTX, spaceID int64, name string) (agentID int64, token string, err error)
	Revoke(ctx context.Context, q db.DBTX, agentID int64, scope auth.Scope) (int64, error)
}

// Admitter checks invite rights and admits invited members into spaces, inside the caller's
// transaction. It is apart from Service so that invites, which admits through it, can be
// built before the Service that takes invites as its revocation hook.
type Admitter struct {
	r           *Resolver
	accounts    memberAccounts
	mailboxes   spaceMailboxes
	now         func() time.Time
	readmission []ReadmissionHook
}

// NewAdmitter returns an Admitter resolving callers with r, minting member tokens through a
// and, when a participating member is invited again, clearing its mailbox in mb and running
// the readmission hooks.
func NewAdmitter(r *Resolver, a memberAccounts, mb spaceMailboxes, now func() time.Time, readmission []ReadmissionHook) *Admitter {
	return &Admitter{r: r, accounts: a, mailboxes: mb, now: now, readmission: readmission}
}

// AuthorizeInvite checks that leader p may invite member name into space and returns the space id.
func (a *Admitter) AuthorizeInvite(ctx context.Context, tx db.DBTX, p auth.Principal, space, name string) (int64, error) {
	m, err := a.r.Resolve(ctx, tx, p, space)
	if err != nil {
		return 0, err
	}
	if err := requireLeaderRole(m, "invite members"); err != nil {
		return 0, err
	}
	if err := a.accounts.CheckMemberName(ctx, tx, name); err != nil {
		return 0, err
	}
	return m.SpaceID, nil
}

// AuthorizeInvites checks that p may list or cancel the invites of space (its leaders and
// admins) and returns the space id.
func (a *Admitter) AuthorizeInvites(ctx context.Context, q db.DBTX, p auth.Principal, space string) (int64, error) {
	m, err := a.r.Resolve(ctx, q, p, space)
	if err != nil {
		return 0, err
	}
	if !m.MaySeeInvites() {
		return 0, apperr.New(apperr.Forbidden, "only leaders may list or cancel invites")
	}
	return m.SpaceID, nil
}

// AdmitMember mints a member token for name in spaceID, adding the participant if new, and
// returns the member's agent id, the space name and the token. A member that already
// participates starts clean first (resetMember).
func (a *Admitter) AdmitMember(ctx context.Context, tx db.DBTX, spaceID int64, name string) (agentID int64, space, token string, err error) {
	if space, err = spaceName(ctx, tx, spaceID); err != nil {
		return 0, "", "", err
	}
	if err := a.resetMember(ctx, tx, spaceID, space, name); err != nil {
		return 0, "", "", err
	}
	agentID, token, err = a.accounts.AdmitMember(ctx, tx, spaceID, name)
	if err != nil {
		return 0, "", "", err
	}
	if err := a.ensureMember(ctx, tx, spaceID, agentID); err != nil {
		return 0, "", "", err
	}
	return agentID, space, token, nil
}

// ensureMember inserts agentID as a member of spaceID unless it already participates.
func (a *Admitter) ensureMember(ctx context.Context, tx db.DBTX, spaceID, agentID int64) error {
	_, found, err := participantRole(ctx, tx, spaceID, agentID)
	if err != nil || found {
		return err
	}
	return insertParticipant(ctx, tx, spaceID, agentID, RoleMember, db.Timestamp(a.now()))
}
