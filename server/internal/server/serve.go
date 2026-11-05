package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/config"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/instance"
)

// Serve builds the bus over conn, listens on cfg.Addr and, unless the GUI is disabled, on
// cfg.WebAddr, publishes the instance under cfg.DataDir() and serves until ctx ends or an
// admin calls POST /v1/admin/shutdown.
func Serve(ctx context.Context, cfg config.Config, conn *sql.DB, adminToken string) error {
	ctx, shutdown := context.WithCancel(ctx)
	defer shutdown()
	streamRoot, cancelStreams := context.WithCancel(context.Background())
	defer cancelStreams()
	h, err := Build(streamRoot, conn, adminToken, shutdown)
	if err != nil {
		return err
	}
	apiLn, webLn, err := listen(cfg.Addr, cfg.WebAddr)
	if err != nil {
		return err
	}
	unpublish, err := instance.Publish(cfg.DataDir(), apiLn.Addr().String(), time.Now())
	if err != nil {
		return errors.Join(err, closeListeners(apiLn, webLn))
	}
	defer unpublish()
	var gui *http.Server
	if webLn != nil {
		gui = NewHTTPServer(h.Web)
	}
	return Run(ctx, NewHTTPServer(h.API), gui, apiLn, webLn, cancelStreams)
}
