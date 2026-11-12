package spaces

import (
	"context"
	"fmt"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// ReadmissionHook drops feature state a member left in a space, such as subscriptions,
// when its name is invited into that space again, so the new agent starts clean. It runs
// inside the admitting transaction, before the new token is issued.
type ReadmissionHook interface {
	OnReadmission(ctx context.Context, tx db.DBTX, m Membership) error
}

// spaceMailboxes deletes an agent's inbox entries in one space.
type spaceMailboxes interface {
	DeleteSpaceEntries(ctx context.Context, q db.DBTX, agentID, spaceID int64) error
}

// resetMember lets member name start clean when it already participates in space spaceID:
// the readmission hooks run, then its inbox entries and its tokens bound to the space are
// deleted. Its participant row and its blackboards stay. Anyone else is left alone.
func (a *Admitter) resetMember(ctx context.Context, tx db.DBTX, spaceID int64, space, name string) error {
	id, role, found, err := participantByName(ctx, tx, spaceID, name)
	if err != nil || !found || role != RoleMember {
		return err
	}
	m := Membership{SpaceID: spaceID, SpaceName: space, AgentID: id, AgentName: name, Role: role}
	for _, h := range a.readmission {
		if err := h.OnReadmission(ctx, tx, m); err != nil {
			return fmt.Errorf("running readmission hook: %w", err)
		}
	}
	if err := a.mailboxes.DeleteSpaceEntries(ctx, tx, id, spaceID); err != nil {
		return err
	}
	_, err = a.accounts.Revoke(ctx, tx, id, auth.InSpace(spaceID))
	return err
}
