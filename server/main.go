// Command agent-message-bus serves the agent message bus over MCP and HTTP.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/blackboards"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/messaging"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/broker"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/config"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpserver"
	"github.com/till-muellers/agent-message-bus/server/internal/server"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
	"github.com/till-muellers/agent-message-bus/server/internal/tools"
	"github.com/till-muellers/agent-message-bus/server/migrations"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	conn, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if err := db.Migrate(ctx, conn, migrations.FS); err != nil {
		return fmt.Errorf("migrate db: %w", err)
	}
	streamRoot, cancelStreams := context.WithCancel(context.Background())
	defer cancelStreams()
	h := build(streamRoot, conn, cfg.AdminToken)
	srv := &http.Server{Addr: cfg.Addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	return httpserver.Serve(ctx, srv, cancelStreams)
}

func build(streamRoot context.Context, conn *sql.DB, adminToken string) http.Handler {
	authSvc := auth.NewService(conn, adminToken, time.Now)
	inboxSvc := inbox.NewService(conn, broker.NewMemory(256), time.Now)
	resolver := spaces.NewResolver(conn)
	msgSvc := messaging.NewService(conn, resolver, inboxSvc, time.Now)
	bbSvc := blackboards.NewService(conn, resolver, inboxSvc, time.Now)
	spaceSvc := spaces.NewService(conn, resolver, authSvc, inboxSvc, time.Now, msgSvc, bbSvc)
	reg := tools.New(tools.Deps{
		Auth: authSvc, Spaces: spaceSvc, Messaging: msgSvc, Blackboards: bbSvc, Inbox: inboxSvc,
	})
	return server.Routes(streamRoot, authSvc, inboxSvc, reg)
}
