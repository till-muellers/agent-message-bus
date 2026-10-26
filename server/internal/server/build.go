package server

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/blackboards"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/invites"
	"github.com/till-muellers/agent-message-bus/server/internal/messaging"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/broker"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
	"github.com/till-muellers/agent-message-bus/server/internal/tools"
)

// Build wires every service onto conn, ensures the bootstrap admin exists and returns the
// routed handler. Inbox streams end when streamRoot is cancelled; shutdown is called by
// POST /v1/admin/shutdown.
func Build(streamRoot context.Context, conn *sql.DB, adminToken string, shutdown func()) (http.Handler, error) {
	authSvc := auth.NewService(conn, adminToken, time.Now)
	if err := authSvc.Bootstrap(streamRoot); err != nil {
		return nil, err
	}
	inboxSvc := inbox.NewService(streamRoot, conn, broker.NewMemory(256), time.Now)
	resolver := spaces.NewResolver(conn)
	msgSvc := messaging.NewService(conn, resolver, inboxSvc, time.Now)
	bbSvc := blackboards.NewService(conn, resolver, inboxSvc, time.Now)
	spaceSvc := spaces.NewService(conn, resolver, authSvc, inboxSvc, time.Now, spaces.Hooks{
		Departure:  []spaces.DepartureHook{msgSvc, bbSvc},
		Creation:   []spaces.CreationHook{msgSvc},
		Revocation: []spaces.RevocationHook{invites.Revocation{}},
	})
	invSvc := invites.NewService(conn, spaceSvc, inboxSvc, time.Now)
	reg := tools.New(tools.Deps{
		Auth: authSvc, Spaces: spaceSvc, Messaging: msgSvc, Blackboards: bbSvc, Inbox: inboxSvc, Invites: invSvc,
	})
	return Routes(streamRoot, authSvc, inboxSvc, invSvc, reg, shutdown), nil
}
