package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpserver"
)

// Run serves api on apiLn and, unless web is nil (GUI disabled), web on webLn until ctx
// ends or either server fails; then both shut down gracefully. cancelStreams ends the API's
// inbox streams. The errors of both servers are joined; the GUI's carry "web listener:".
func Run(ctx context.Context, api, web *http.Server, apiLn, webLn net.Listener, cancelStreams context.CancelFunc) error {
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	var wg sync.WaitGroup
	var apiErr, webErr error
	wg.Go(func() {
		defer stop()
		apiErr = httpserver.Serve(ctx, api, apiLn, cancelStreams)
	})
	if web != nil {
		wg.Go(func() {
			defer stop()
			if err := httpserver.Serve(ctx, web, webLn, func() {}); err != nil {
				webErr = fmt.Errorf("web listener: %w", err)
			}
		})
	}
	wg.Wait()
	return errors.Join(apiErr, webErr)
}
