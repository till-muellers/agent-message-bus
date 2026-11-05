package spaces

import (
	"context"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// memberAccounts vets member names and mints the member tokens an Admitter hands out.
type memberAccounts interface {
	CheckMemberName(ctx context.Context, q db.DBTX, name string) error
	AdmitMember(ctx context.Context, tx db.DBTX, spaceID int64, name string) (agentID int64, token string, err error)
}

// Admitter checks invite rights and admits invited members into spaces, inside the caller's
// transaction. It is apart from Service so that invites, which admits through it, can be
// built before the Service that takes invites as its revocation hook.
type Admitter struct {
	r        *Resolver
	accounts memberAccounts
	now      func() time.Time
}

// NewAdmitter returns an Admitter resolving callers with r and minting member tokens through a.
func NewAdmitter(r *Resolver, a memberAccounts, now func() time.Time) *Admitter {
	return &Admitter{r: r, accounts: a, now: now}
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
// returns the member's agent id, the space name and the token.
func (a *Admitter) AdmitMember(ctx context.Context, tx db.DBTX, spaceID int64, name string) (agentID int64, space, token string, err error) {
	agentID, token, err = a.accounts.AdmitMember(ctx, tx, spaceID, name)
	if err != nil {
		return 0, "", "", err
	}
	if err := a.ensureMember(ctx, tx, spaceID, agentID); err != nil {
		return 0, "", "", err
	}
	if space, err = spaceName(ctx, tx, spaceID); err != nil {
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
