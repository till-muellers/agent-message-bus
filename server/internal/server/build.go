package server

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/blackboards"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/messaging"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/broker"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
	"github.com/till-muellers/agent-message-bus/server/internal/tools"
)

// Build wires every service onto conn and returns the routed handler.
// Inbox streams end when streamRoot is cancelled.
func Build(streamRoot context.Context, conn *sql.DB, adminToken string) http.Handler {
	authSvc := auth.NewService(conn, adminToken, time.Now)
	inboxSvc := inbox.NewService(conn, broker.NewMemory(256), time.Now)
	resolver := spaces.NewResolver(conn)
	msgSvc := messaging.NewService(conn, resolver, inboxSvc, time.Now)
	bbSvc := blackboards.NewService(conn, resolver, inboxSvc, time.Now)
	spaceSvc := spaces.NewService(conn, resolver, authSvc, inboxSvc, time.Now,
		[]spaces.DepartureHook{msgSvc, bbSvc}, []spaces.CreationHook{msgSvc})
	reg := tools.New(tools.Deps{
		Auth: authSvc, Spaces: spaceSvc, Messaging: msgSvc, Blackboards: bbSvc, Inbox: inboxSvc,
	})
	return Routes(streamRoot, authSvc, inboxSvc, reg)
}
