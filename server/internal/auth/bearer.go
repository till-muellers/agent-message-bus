package auth

import (
	"net/http"
	"strings"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

// BearerToken returns the token of "Authorization: Bearer <token>", or Unauthenticated when
// the header is missing, uses another scheme or is not exactly scheme and token. The scheme
// matches in any case and may be followed by any whitespace (RFC 7235, as the MCP SDK does).
func BearerToken(r *http.Request) (string, error) {
	f := strings.Fields(r.Header.Get("Authorization"))
	if len(f) != 2 || !strings.EqualFold(f[0], "Bearer") {
		return "", apperr.New(apperr.Unauthenticated, "bearer token required")
	}
	return f[1], nil
}
