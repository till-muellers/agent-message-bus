package auth

import (
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

// MinAdminTokenLen is the minimum length of an admin token.
const MinAdminTokenLen = 32

// SetAdminToken atomically replaces the admin credential; the old token stops working.
func (s *Service) SetAdminToken(token string) error {
	if len(token) < MinAdminTokenLen {
		return apperr.New(apperr.Invalid, "admin token must be at least %d characters", MinAdminTokenLen)
	}
	h := Hash(token)
	s.adminHash.Store(&h)
	return nil
}
