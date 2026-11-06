package spaces

import (
	"context"
	"fmt"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// Revoke removes member name from space, if present, and runs the revocation hooks,
// which cancel state such as pending invite codes. It reports whether anything was removed
// and wakes the removed member's streams and waits to re-check their token.
// Leader targets are refused before any hook runs.
func (s *Service) Revoke(ctx context.Context, p auth.Principal, space, name string) (bool, error) {
	var deleted bool
	var target int64
	_, err := inbox.InTx(ctx, s.db, s.ib, func(tx db.DBTX) ([]inbox.Entry, error) {
		entries, removed, id, err := s.revokeTx(ctx, tx, p, space, name)
		deleted, target = removed, id
		return entries, err
	})
	if err != nil {
		return false, err
	}
	if target != 0 {
		s.ib.Wake(target)
	}
	return deleted, nil
}

// revokeTx authorizes leader p, checks the target, runs the hooks and removes the participant.
// It returns the removed participant's agent id, 0 when none was removed.
func (s *Service) revokeTx(ctx context.Context, tx db.DBTX, p auth.Principal, space, name string) ([]inbox.Entry, bool, int64, error) {
	m, err := s.r.Resolve(ctx, tx, p, space)
	if err != nil {
		return nil, false, 0, err
	}
	if err := requireLeaderRole(m, "revoke members"); err != nil {
		return nil, false, 0, err
	}
	target, found, err := revocableMember(ctx, tx, m, name)
	if err != nil {
		return nil, false, 0, err
	}
	cancelled, err := s.runRevocationHooks(ctx, tx, m.SpaceID, name)
	if err != nil {
		return nil, false, 0, err
	}
	if !found {
		return nil, cancelled, 0, nil
	}
	entries, err := s.removeParticipant(ctx, tx, target)
	if err != nil {
		return nil, false, 0, err
	}
	return entries, true, target.AgentID, nil
}

// runRevocationHooks runs every revocation hook and reports whether any removed something.
func (s *Service) runRevocationHooks(ctx context.Context, tx db.DBTX, spaceID int64, name string) (bool, error) {
	var cancelled bool
	for _, h := range s.hooks.Revocation {
		removed, err := h.OnRevoke(ctx, tx, spaceID, name)
		if err != nil {
			return false, fmt.Errorf("running revocation hook: %w", err)
		}
		cancelled = cancelled || removed
	}
	return cancelled, nil
}

// revocableMember looks up participant name in the space of m. Admins may also revoke
// leaders; nobody revokes an admin with it.
func revocableMember(ctx context.Context, tx db.DBTX, m Membership, name string) (Membership, bool, error) {
	id, role, found, err := participantByName(ctx, tx, m.SpaceID, name)
	if err != nil || !found {
		return Membership{}, false, err
	}
	if err := checkRevocable(m, name, role); err != nil {
		return Membership{}, false, err
	}
	return Membership{SpaceID: m.SpaceID, SpaceName: m.SpaceName, AgentID: id, AgentName: name, Role: role}, true, nil
}

// checkRevocable refuses admin targets, and leader targets unless the revoker m is an admin.
func checkRevocable(m Membership, name string, role Role) error {
	switch {
	case role == RoleAdmin:
		return apperr.New(apperr.Conflict, "%s is an admin of space %s; admins cannot be revoked from a space", name, m.SpaceName)
	case role == RoleLeader && !m.Admin:
		return apperr.New(apperr.Conflict, "%s is a leader of space %s; leaders cannot be revoked", name, m.SpaceName)
	}
	return nil
}
