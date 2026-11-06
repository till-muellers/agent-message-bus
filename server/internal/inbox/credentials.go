package inbox

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// credentials re-validates the token a caller authenticated with (auth.Service.StillValid):
// a stream or a Wait outlives the request check, so it asks again before it hands out mail.
type credentials interface {
	StillValid(ctx context.Context, q db.DBTX, p auth.Principal) error
}

// stillValid fails with Unauthenticated once the token behind p was revoked, rotated or
// deleted with its space.
func (s *Service) stillValid(ctx context.Context, p auth.Principal) error {
	return s.creds.StillValid(ctx, s.db, p)
}

// isRevoked reports whether err is the Unauthenticated failure of stillValid.
func isRevoked(err error) bool {
	kind, ok := apperr.KindOf(err)
	return ok && kind == apperr.Unauthenticated
}
