package spaces

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

// Revoke removes member name from space, if present, and runs the revocation hooks,
// which cancel state such as pending invite codes. It reports whether anything was removed.
// Leader targets are refused before any hook runs.
func (s *Service) Revoke(ctx context.Context, p auth.Principal, space, name string) (bool, error) {
	var deleted bool
	err := s.ib.Deliver(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
		entries, removed, err := s.revokeTx(ctx, tx, p, space, name)
		deleted = removed
		return entries, err
	})
	if err != nil {
		return false, err
	}
	return deleted, nil
}

// revokeTx authorizes leader p, checks the target, runs the hooks and removes the participant.
func (s *Service) revokeTx(ctx context.Context, tx *sql.Tx, p auth.Principal, space, name string) ([]inbox.Entry, bool, error) {
	m, err := s.r.ResolveTx(ctx, tx, p, space)
	if err != nil {
		return nil, false, err
	}
	if err := requireLeaderRole(m, "revoke members"); err != nil {
		return nil, false, err
	}
	target, found, err := revocableMember(ctx, tx, m, name)
	if err != nil {
		return nil, false, err
	}
	cancelled, err := s.runRevocationHooks(ctx, tx, m.SpaceID, name)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, cancelled, nil
	}
	entries, err := s.removeParticipant(ctx, tx, target)
	if err != nil {
		return nil, false, err
	}
	return entries, true, nil
}

// runRevocationHooks runs every revocation hook and reports whether any removed something.
func (s *Service) runRevocationHooks(ctx context.Context, tx *sql.Tx, spaceID int64, name string) (bool, error) {
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

// revocableMember looks up member name in the space of leader m; leaders cannot be revoked.
func revocableMember(ctx context.Context, tx *sql.Tx, m Membership, name string) (Membership, bool, error) {
	id, role, found, err := participantByName(ctx, tx, m.SpaceID, name)
	if err != nil || !found {
		return Membership{}, false, err
	}
	if role != RoleMember {
		return Membership{}, false, apperr.New(apperr.Conflict, "%s is a leader of space %s; leaders cannot be revoked", name, m.SpaceName)
	}
	return Membership{SpaceID: m.SpaceID, SpaceName: m.SpaceName, AgentID: id, AgentName: name, Role: role}, true, nil
}
