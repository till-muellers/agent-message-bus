package auth

import (
	"net/http"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpx"
)

// Middleware requires "Authorization: Bearer <token>" and stores the Principal in the context.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := BearerToken(r)
		if err != nil {
			httpx.WriteError(w, err)
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
