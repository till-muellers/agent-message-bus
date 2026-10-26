// Command agent-message-bus serves the agent message bus over MCP and HTTP and manages its local instance.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/config"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/detach"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpserver"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/instance"
	"github.com/till-muellers/agent-message-bus/server/internal/server"
	"github.com/till-muellers/agent-message-bus/server/migrations"
)

const usage = `Usage: agent-message-bus [command]

Serves the agent message bus (MCP at /mcp, RPC at /v1/tools, SSE at /v1/inbox/stream, /healthz).
One instance runs per data dir.

Commands:
  (none)          serve in the foreground; Ctrl+C or SIGTERM shuts down gracefully
  start           start in the background (output appends to <datadir>/agent-message-bus.log)
                  and wait until healthy; no-op when already running
  stop            shut the running instance down (POST /v1/admin/shutdown, kill on timeout)
  status [--json] print whether an instance is running, with its pid and url
  admin-token     print the bootstrap admin's token (generates <datadir>/admin-token if missing)
  rotate-token    generate a new admin token, apply it to the running instance and save it

Flags:
  --help          show this help

Environment:
  AMB_ADMIN_TOKEN  optional admin token override (at least 32 characters); default is the
                   generated <datadir>/admin-token file
  AMB_ADDR         listen address (default 127.0.0.1:8080)
  AMB_DB_PATH      SQLite file (default <datadir>/agent-message-bus.db, where <datadir> is
                   Windows: %LOCALAPPDATA%\agent-message-bus
                   macOS:   ~/Library/Application Support/agent-message-bus
                   others:  $XDG_DATA_HOME/agent-message-bus or ~/.local/share/agent-message-bus)
                   The state file agent-message-bus.state.json lives next to it.
`

const waitTimeout = 10 * time.Second

func main() {
	fs := flag.NewFlagSet("agent-message-bus", flag.ExitOnError)
	fs.Usage = func() { _, _ = fmt.Fprint(fs.Output(), usage) }
	_ = fs.Parse(os.Args[1:]) // ExitOnError: --help exits 0, bad flags exit 2
	if err := dispatch(fs.Args()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func dispatch(args []string) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if len(args) == 0 {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
		return run(ctx, cfg)
	}
	return command(ctx, manager(cfg), args[0], args[1:])
}

func command(ctx context.Context, m instance.Manager, name string, rest []string) error {
	switch name {
	case "start":
		return m.Start(ctx)
	case "stop":
		return m.Stop(ctx)
	case "status":
		fs := flag.NewFlagSet("status", flag.ExitOnError)
		asJSON := fs.Bool("json", false, "print JSON")
		_ = fs.Parse(rest)
		return m.Status(ctx, *asJSON)
	case "admin-token":
		return m.AdminToken()
	case "rotate-token":
		return m.RotateToken(ctx)
	}
	return fmt.Errorf("unknown command %q; run agent-message-bus --help", name)
}

func manager(cfg config.Config) instance.Manager {
	return instance.Manager{
		Config:  cfg,
		Client:  &http.Client{Timeout: 5 * time.Second},
		Spawn:   func(logPath string) (int, error) { return detach.Start(nil, os.Environ(), logPath) },
		Kill:    instance.KillPID,
		Out:     os.Stdout,
		Timeout: waitTimeout,
		Poll:    100 * time.Millisecond,
	}
}

func run(ctx context.Context, cfg config.Config) error {
	token, err := cfg.ResolveAdminToken()
	if err != nil {
		return fmt.Errorf("admin token: %w", err)
	}
	if err := instance.EnsureNotRunning(ctx, http.DefaultClient, cfg.DataDir()); err != nil {
		return err
	}
	conn, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if err := db.Migrate(ctx, conn, migrations.FS); err != nil {
		return fmt.Errorf("migrate db: %w", err)
	}
	return serve(ctx, cfg, conn, token)
}

func serve(ctx context.Context, cfg config.Config, conn *sql.DB, token string) error {
	ctx, shutdown := context.WithCancel(ctx)
	defer shutdown()
	streamRoot, cancelStreams := context.WithCancel(context.Background())
	defer cancelStreams()
	handler, err := server.Build(streamRoot, conn, token, shutdown)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.Addr, err)
	}
	unpublish, err := instance.Publish(cfg.DataDir(), ln.Addr().String(), time.Now())
	if err != nil {
		return errors.Join(err, ln.Close())
	}
	defer unpublish()
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	return httpserver.Serve(ctx, srv, ln, cancelStreams)
}
