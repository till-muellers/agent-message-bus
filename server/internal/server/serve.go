package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/config"
)

// Publisher is the serving process's claim on its data dir (instance.Lock): the instance id
// /healthz names and the state file it publishes once the API listens.
type Publisher interface {
	ID() string
	Publish(addr string, now time.Time) (unpublish func() error, err error)
}

// Serve builds the bus over conn, listens on cfg.Addr and, unless the GUI is disabled, on
// cfg.WebAddr, publishes the instance through claim and serves until ctx ends or an admin
// calls POST /v1/admin/shutdown.
func Serve(ctx context.Context, cfg config.Config, conn *sql.DB, adminToken string, claim Publisher) error {
	ctx, shutdown := context.WithCancel(ctx)
	defer shutdown()
	streamRoot, cancelStreams := context.WithCancel(context.Background())
	defer cancelStreams()
	h, err := Build(streamRoot, conn, adminToken, claim.ID(), shutdown)
	if err != nil {
		return err
	}
	apiLn, webLn, err := listen(ctx, cfg.Addr, cfg.WebAddr)
	if err != nil {
		return err
	}
	unpublish, err := claim.Publish(apiLn.Addr().String(), time.Now())
	if err != nil {
		return errors.Join(err, closeListeners(apiLn, webLn))
	}
	var gui *http.Server
	if webLn != nil {
		gui = NewHTTPServer(h.Web)
	}
	return errors.Join(Run(ctx, NewHTTPServer(h.API), gui, apiLn, webLn, cancelStreams), unpublish())
}
