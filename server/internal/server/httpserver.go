package server

import (
	"net/http"
	"time"
)

// NewHTTPServer returns the server for h, used for the API and for the web GUI listener. It
// sets only a read-header timeout: no read, write or idle timeout may cut a long bus_wait
// call or an inbox stream. A body the handler left unread is drained (drainUnreadBody).
func NewHTTPServer(h http.Handler) *http.Server {
	return &http.Server{Handler: drainUnreadBody(h), ReadHeaderTimeout: 10 * time.Second}
}
