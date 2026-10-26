package spaces

import (
	"context"
	"database/sql"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

// RevokeAdmin deletes every token of minted admin name and removes it from every space it
// joined, in one transaction; spaces left empty are deleted. The agent row stays, so
// minting the name again gives the same agent. It reports whether anything was removed
// and logs "admin revoked" after commit. Admins only; the bootstrap admin cannot be revoked.
func (s *Service) RevokeAdmin(ctx context.Context, p auth.Principal, name string) (bool, error) {
	if err := auth.RequireAdmin(p); err != nil {
		return false, err
	}
	var deleted bool
	err := s.ib.Deliver(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
		entries, removed, err := s.revokeAdminTx(ctx, tx, name)
		deleted = removed
		return entries, err
	})
	if err != nil {
		return false, err
	}
	s.log.InfoContext(ctx, "admin revoked", "by", p.AgentName, "name", name, "deleted", deleted)
	return deleted, nil
}

// revokeAdminTx removes admin name's tokens and participations and returns the departure entries.
func (s *Service) revokeAdminTx(ctx context.Context, tx *sql.Tx, name string) ([]inbox.Entry, bool, error) {
	id, found, bootstrap, err := s.accounts.AdminAgent(ctx, tx, name)
	if err != nil || !found {
		return nil, false, err
	}
	if bootstrap {
		return nil, false, apperr.New(apperr.Forbidden,
			"admin is the bootstrap admin; rotate its token with agent-message-bus rotate-token")
	}
	tokens, err := s.accounts.RevokeAgentTokens(ctx, tx, id)
	if err != nil {
		return nil, false, err
	}
	entries, departed, err := s.departEverywhere(ctx, tx, Membership{AgentID: id, AgentName: name, Role: RoleAdmin, Admin: true})
	return entries, tokens > 0 || departed > 0, err
}

// departEverywhere removes agent m from every space it joined and reports how many it left.
func (s *Service) departEverywhere(ctx context.Context, tx *sql.Tx, m Membership) ([]inbox.Entry, int, error) {
	rows, err := joinedSpaces(ctx, tx, m.AgentID)
	if err != nil {
		return nil, 0, err
	}
	var entries []inbox.Entry
	for _, row := range rows {
		m.SpaceID, m.SpaceName = row.id, row.name
		es, err := s.removeParticipant(ctx, tx, m)
		if err != nil {
			return nil, 0, err
		}
		entries = append(entries, es...)
	}
	return entries, len(rows), nil
}
