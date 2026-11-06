package auth

import (
	"github.com/till-muellers/agent-message-bus/server/internal/domain"
)

// SetAdminToken atomically replaces the admin credential with token, trimmed; the old token
// stops working. A token domain.AdminToken rejects leaves the old one in place.
func (s *Service) SetAdminToken(token string) error {
	token, err := domain.AdminToken(token)
	if err != nil {
		return err
	}
	h := Hash(token)
	s.adminHash.Store(&h)
	return nil
}
