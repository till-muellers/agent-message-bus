package auth

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// StillValid re-validates p through q, inside the caller's write transaction: the token
// that authenticated p must still exist, and for the bootstrap admin still be the instance
// admin token. A revocation or rotation that committed after p was authenticated fails the
// call with Unauthenticated "invalid token", so the revocation wins over calls in flight.
func (s *Service) StillValid(ctx context.Context, q db.DBTX, p Principal) error {
	valid, err := s.credentialValid(ctx, q, p)
	if err != nil {
		return err
	}
	if !valid {
		return errInvalidToken()
	}
	return nil
}

// credentialValid reports whether the token that authenticated p still authenticates.
func (s *Service) credentialValid(ctx context.Context, q db.DBTX, p Principal) (bool, error) {
	if p.Bootstrap {
		return s.isAdminHash(p.credential), nil
	}
	return tokenExists(ctx, q, p.credential)
}
