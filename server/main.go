// Command agent-message-bus serves the agent message bus over MCP and HTTP.
package main

import (
	"context"
	"database/sql"
	"errors"
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
	"github.com/till-muellers/agent-message-bus/server/internal/server"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
	"github.com/till-muellers/agent-message-bus/server/internal/tools"
	"github.com/till-muellers/agent-message-bus/server/migrations"
)

const shutdownTimeout = 10 * time.Second

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
	defer conn.Close()
	if err := db.Migrate(ctx, conn, migrations.FS); err != nil {
		return fmt.Errorf("migrate db: %w", err)
	}
	streamRoot, cancelStreams := context.WithCancel(context.Background())
	defer cancelStreams()
	h := build(streamRoot, conn, cfg.AdminToken)
	srv := &http.Server{Addr: cfg.Addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	return serve(ctx, srv, cancelStreams)
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

func serve(ctx context.Context, srv *http.Server, cancelStreams context.CancelFunc) error {
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	slog.Info("listening", "addr", srv.Addr)
	select {
	case err := <-errc:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}
	slog.Info("shutting down")
	cancelStreams()
	sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
