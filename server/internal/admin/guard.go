// Package admin serves the localhost-only instance management endpoints.
package admin

import (
	"net"
	"net/http"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpx"
)

// guard admits only the admin principal calling from a loopback address. Run it behind auth.Middleware.
func guard(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopback(r.RemoteAddr) {
			httpx.WriteError(w, apperr.New(apperr.Forbidden, "admin endpoints are only available from localhost"))
			return
		}
		if p, ok := auth.FromContext(r.Context()); !ok || p.Kind != auth.Admin {
			httpx.WriteError(w, apperr.New(apperr.Forbidden, "admin token required"))
			return
		}
		next(w, r)
	})
}

func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
