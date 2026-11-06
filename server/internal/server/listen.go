package server

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// listen opens the API listener on apiAddr and, unless webAddr is empty (GUI disabled), the
// GUI listener on webAddr; when the GUI listener fails, the API listener is closed again. Both
// bind exclusively (listenConfig), so a GUI address overlapping the API's on the same port
// fails instead of shadowing it.
func listen(ctx context.Context, apiAddr, webAddr string) (api, web net.Listener, err error) {
	lc := listenConfig()
	api, err = lc.Listen(ctx, "tcp", apiAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("listen %s: %w", apiAddr, err)
	}
	if webAddr == "" {
		return api, nil, nil
	}
	web, err = lc.Listen(ctx, "tcp", webAddr)
	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("listen AMB_WEB_ADDR %s: %w", webAddr, err), api.Close())
	}
	return api, web, nil
}

// closeListeners closes api and, when the GUI is enabled, web.
func closeListeners(api, web net.Listener) error {
	if web == nil {
		return api.Close()
	}
	return errors.Join(api.Close(), web.Close())
}
