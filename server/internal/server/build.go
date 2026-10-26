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
	"github.com/till-muellers/agent-message-bus/server/internal/overview"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/broker"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
	"github.com/till-muellers/agent-message-bus/server/internal/tools"
	"github.com/till-muellers/agent-message-bus/server/internal/web"
)

// Handlers are the two listeners' handlers: the API (AMB_ADDR) and the web GUI (AMB_WEB_ADDR).
type Handlers struct{ API, Web http.Handler }

// Build wires every service onto conn, ensures the bootstrap admin exists and returns the
// routed handlers. Inbox streams end when streamRoot is cancelled; shutdown is called by
// POST /v1/admin/shutdown.
func Build(streamRoot context.Context, conn *sql.DB, adminToken string, shutdown func()) (Handlers, error) {
	authSvc := auth.NewService(conn, adminToken, time.Now)
	if err := authSvc.Bootstrap(streamRoot); err != nil {
		return Handlers{}, err
	}
	dist, err := web.Dist()
	if err != nil {
		return Handlers{}, err
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
	ov := overview.NewService(resolver, spaceSvc, msgSvc, bbSvc)
	reg := tools.New(tools.Deps{
		Auth: authSvc, Spaces: spaceSvc, Messaging: msgSvc, Blackboards: bbSvc, Inbox: inboxSvc, Invites: invSvc,
		Overview: ov,
	})
	gui := web.New(web.Deps{Auth: authSvc, Spaces: spaceSvc, Overview: ov, Invites: invSvc, Inbox: inboxSvc,
		Tools: reg, Dist: dist, Now: time.Now})
	return Handlers{API: Routes(streamRoot, authSvc, inboxSvc, invSvc, reg, shutdown), Web: WebRoutes(gui)}, nil
}
