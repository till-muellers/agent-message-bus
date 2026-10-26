package spaces

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// AuthorizeInvite checks that leader p may invite member name into space and returns the space id.
func (s *Service) AuthorizeInvite(ctx context.Context, tx db.DBTX, p auth.Principal, space, name string) (int64, error) {
	m, err := s.r.ResolveTx(ctx, tx, p, space)
	if err != nil {
		return 0, err
	}
	if err := requireLeaderRole(m, "invite members"); err != nil {
		return 0, err
	}
	if err := s.accounts.CheckMemberName(ctx, tx, name); err != nil {
		return 0, err
	}
	return m.SpaceID, nil
}

// AuthorizeInvites checks that p may list or cancel the invites of space (its leaders and
// admins) and returns the space id.
func (s *Service) AuthorizeInvites(ctx context.Context, q db.DBTX, p auth.Principal, space string) (int64, error) {
	m, err := s.r.ResolveTx(ctx, q, p, space)
	if err != nil {
		return 0, err
	}
	if err := requireLeaderRole(m, "list or cancel invites"); err != nil {
		return 0, err
	}
	return m.SpaceID, nil
}

// AdmitMember mints a member token for name in spaceID, adding the participant if new, and
// returns the member's agent id, the space name and the token.
func (s *Service) AdmitMember(ctx context.Context, tx db.DBTX, spaceID int64, name string) (agentID int64, space, token string, err error) {
	agentID, token, err = s.accounts.MintMember(ctx, tx, name, spaceID)
	if err != nil {
		return 0, "", "", err
	}
	if err := s.ensureMember(ctx, tx, spaceID, agentID); err != nil {
		return 0, "", "", err
	}
	if space, err = spaceName(ctx, tx, spaceID); err != nil {
		return 0, "", "", err
	}
	return agentID, space, token, nil
}

// ensureMember inserts agentID as a member of spaceID unless it already participates.
func (s *Service) ensureMember(ctx context.Context, tx db.DBTX, spaceID, agentID int64) error {
	_, found, err := participantRole(ctx, tx, spaceID, agentID)
	if err != nil || found {
		return err
	}
	return insertParticipant(ctx, tx, spaceID, agentID, RoleMember, s.stamp())
}
