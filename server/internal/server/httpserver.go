package server

import (
	"net/http"
	"time"
)

// NewHTTPServer returns the server for h, used for the API and for the web GUI listener. It
// sets only a read-header timeout: no read, write or idle timeout may cut a long bus_wait
// call or an inbox stream.
func NewHTTPServer(h http.Handler) *http.Server {
	return &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
}
