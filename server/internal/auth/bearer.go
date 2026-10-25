package auth

import (
	"net/http"
	"strings"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

// BearerToken returns the non-empty token of "Authorization: Bearer <token>",
// or Unauthenticated when the header is missing, empty or uses another scheme.
func BearerToken(r *http.Request) (string, error) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return "", apperr.New(apperr.Unauthenticated, "bearer token required")
	}
	return token, nil
}
