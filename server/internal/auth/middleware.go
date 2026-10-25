package auth

import (
	"net/http"
	"strings"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpx"
)

// Middleware requires "Authorization: Bearer <token>" and stores the Principal in the context.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			httpx.WriteError(w, apperr.New(apperr.Unauthenticated, "bearer token required"))
			return
		}
		p, err := s.Authenticate(r.Context(), token)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
	})
}
