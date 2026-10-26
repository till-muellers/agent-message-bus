package auth

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// BootstrapName is the agent the instance admin token authenticates as.
const BootstrapName = "admin"

// Bootstrap ensures the admin agent "admin" exists and remembers its id for the
// instance admin token. An existing agent "admin" of another kind is a Conflict.
func (s *Service) Bootstrap(ctx context.Context) error {
	var id int64
	err := db.InTx(ctx, s.db, func(tx *sql.Tx) error {
		var err error
		id, err = s.ensureAgent(ctx, tx, BootstrapName, Admin)
		return err
	})
	if err != nil {
		return fmt.Errorf("bootstrapping admin agent: %w", err)
	}
	s.bootstrapID.Store(id)
	return nil
}

// bootstrapPrincipal is the caller of the instance admin token.
func (s *Service) bootstrapPrincipal() Principal {
	return Principal{Kind: Admin, AgentID: s.bootstrapID.Load(), AgentName: BootstrapName, Bootstrap: true}
}
