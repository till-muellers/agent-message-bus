// Command agent-message-bus serves the agent message bus over MCP and HTTP.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/config"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/detach"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpserver"
	"github.com/till-muellers/agent-message-bus/server/internal/server"
	"github.com/till-muellers/agent-message-bus/server/migrations"
)

const usage = `Usage: agent-message-bus [--detached]

Serves the agent message bus (MCP at /mcp, RPC at /v1/tools, SSE at /v1/inbox/stream, /healthz).

Flags:
  --detached   run in the background; output appends to <db dir>/agent-message-bus.log,
               pid is written to <db dir>/agent-message-bus.pid
  --help       show this help

Environment:
  AMB_ADMIN_TOKEN  required, at least 32 characters
  AMB_ADDR         listen address (default :8080)
  AMB_DB_PATH      SQLite file (default <datadir>/agent-message-bus.db, where <datadir> is
                   Windows: %LOCALAPPDATA%\agent-message-bus
                   macOS:   ~/Library/Application Support/agent-message-bus
                   others:  $XDG_DATA_HOME/agent-message-bus or ~/.local/share/agent-message-bus)
`

func main() {
	fs := flag.NewFlagSet("agent-message-bus", flag.ExitOnError)
	fs.Usage = func() { fmt.Fprint(fs.Output(), usage) }
	detached := fs.Bool("detached", false, "run in the background")
	_ = fs.Parse(os.Args[1:]) // ExitOnError: --help exits 0, bad flags exit 2
	if err := dispatch(*detached); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func dispatch(detached bool) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if detached {
		return startDetached(cfg)
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return run(ctx, cfg)
}

func startDetached(cfg config.Config) error {
	dir := cfg.DataDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating data dir %s: %w", dir, err)
	}
	logPath := filepath.Join(dir, "agent-message-bus.log")
	pidPath := filepath.Join(dir, "agent-message-bus.pid")
	pid, err := detach.Start(os.Args[1:], os.Environ(), logPath, pidPath)
	if err != nil {
		return fmt.Errorf("detach: %w", err)
	}
	fmt.Printf("agent-message-bus started (pid %d); log: %s; pid file: %s\n", pid, logPath, pidPath)
	return nil
}

func run(ctx context.Context, cfg config.Config) error {
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
	h := server.Build(streamRoot, conn, cfg.AdminToken)
	srv := &http.Server{Addr: cfg.Addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	return httpserver.Serve(ctx, srv, cancelStreams)
}
