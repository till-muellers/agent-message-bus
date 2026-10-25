package invites

import (
	"net/http"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpx"
)

// AcceptHandler serves POST /v1/invites/accept: the invite code is the bearer token.
func (s *Service) AcceptHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code, err := auth.BearerToken(r)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		a, err := s.Accept(r.Context(), code)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, a)
	})
}
