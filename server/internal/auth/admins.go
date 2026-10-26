package auth

import (
	"context"
	"database/sql"

	"github.com/till-muellers/agent-message-bus/server/internal/domain"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// AdminInfo describes one admin agent for bus_list_admins.
type AdminInfo struct {
	Name      string   `json:"name"`
	Bootstrap bool     `json:"bootstrap"`
	Tokens    int      `json:"tokens"`
	Spaces    []string `json:"spaces"`
}

// RequireAdmin fails with Forbidden unless p is an admin.
func RequireAdmin(p Principal) error {
	if p.Kind != Admin {
		return apperr.New(apperr.Forbidden, "admin token required")
	}
	return nil
}

// MintAdmin creates (or reuses) admin agent name and returns a new amb_admin_ token for it.
// Admins only; the bootstrap name is reserved. Logs "admin minted" after commit.
func (s *Service) MintAdmin(ctx context.Context, p Principal, name string) (string, error) {
	if err := RequireAdmin(p); err != nil {
		return "", err
	}
	if err := domain.ValidName("name", name); err != nil {
		return "", err
	}
	if name == BootstrapName {
		return "", apperr.New(apperr.Conflict, "admin is the bootstrap admin; its token is the instance admin token")
	}
	var token string
	err := db.InTx(ctx, s.db, func(tx *sql.Tx) error {
		id, err := s.ensureAgent(ctx, tx, name, Admin)
		if err != nil {
			return err
		}
		token, err = s.issue(ctx, tx, AdminTokenPrefix, id, 0)
		return err
	})
	if err != nil {
		return "", err
	}
	s.log.InfoContext(ctx, "admin minted", "by", p.AgentName, "name", name)
	return token, nil
}

// ListAdmins returns every admin agent sorted by name, with its token count and joined spaces.
func (s *Service) ListAdmins(ctx context.Context, p Principal) ([]AdminInfo, error) {
	if err := RequireAdmin(p); err != nil {
		return nil, err
	}
	admins, err := listAdmins(ctx, s.db)
	if err != nil {
		return nil, err
	}
	for i := range admins {
		admins[i].Bootstrap = admins[i].Name == BootstrapName
	}
	return admins, nil
}

// AdminAgent looks up admin agent name through q. An agent of another kind is a Conflict;
// bootstrap reports whether name is the bootstrap admin.
func (s *Service) AdminAgent(ctx context.Context, q db.DBTX, name string) (id int64, found, bootstrap bool, err error) {
	id, found, err = s.agentOfKind(ctx, q, name, Admin)
	return id, found, found && name == BootstrapName, err
}

// RevokeAgentTokens deletes every token of agentID through q and reports how many were removed.
func (s *Service) RevokeAgentTokens(ctx context.Context, q db.DBTX, agentID int64) (int64, error) {
	return deleteAgentTokens(ctx, q, agentID)
}
