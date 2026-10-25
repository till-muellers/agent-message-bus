package auth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"sync/atomic"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/domain"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// Service authenticates bearer tokens and mints leader and member tokens.
type Service struct {
	db        *sql.DB
	adminHash atomic.Pointer[string]
	now       func() time.Time
}

// NewService returns a Service using adminToken as the admin credential.
func NewService(db *sql.DB, adminToken string, now func() time.Time) *Service {
	s := &Service{db: db, now: now}
	h := Hash(adminToken)
	s.adminHash.Store(&h)
	return s
}

// Authenticate resolves token to a Principal.
func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	h := Hash(token)
	if subtle.ConstantTimeCompare([]byte(h), []byte(*s.adminHash.Load())) == 1 {
		return Principal{Kind: Admin}, nil
	}
	p, found, err := lookupToken(ctx, s.db, h)
	if err != nil {
		return Principal{}, err
	}
	if !found {
		return Principal{}, apperr.New(apperr.Unauthenticated, "invalid token")
	}
	return p, nil
}

// MintLeader creates (or reuses) leader agent name and returns a new token. Admin only.
func (s *Service) MintLeader(ctx context.Context, p Principal, name string) (string, error) {
	if p.Kind != Admin {
		return "", apperr.New(apperr.Forbidden, "admin token required")
	}
	if err := domain.ValidName("name", name); err != nil {
		return "", err
	}
	var token string
	err := db.InTx(ctx, s.db, func(tx *sql.Tx) error {
		id, err := s.ensureAgent(ctx, tx, name, Leader)
		if err != nil {
			return err
		}
		token, err = s.issue(ctx, tx, id, 0)
		return err
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// MintMember creates (or reuses) member agent name and issues a token bound to spaceID.
func (s *Service) MintMember(ctx context.Context, tx db.DBTX, name string, spaceID int64) (agentID int64, token string, err error) {
	if err := domain.ValidName("name", name); err != nil {
		return 0, "", err
	}
	agentID, err = s.ensureAgent(ctx, tx, name, Member)
	if err != nil {
		return 0, "", err
	}
	token, err = s.issue(ctx, tx, agentID, spaceID)
	if err != nil {
		return 0, "", err
	}
	return agentID, token, nil
}

// ensureAgent returns the id of agent name with kind, creating it if missing.
func (s *Service) ensureAgent(ctx context.Context, q db.DBTX, name string, kind Kind) (int64, error) {
	id, existing, found, err := findAgent(ctx, q, name)
	if err != nil {
		return 0, err
	}
	if !found {
		return insertAgent(ctx, q, name, kind, s.stamp())
	}
	if existing != kind {
		return 0, apperr.New(apperr.Conflict, "agent %s is a %s agent", name, existing)
	}
	return id, nil
}

// issue stores a fresh token's hash and returns the plaintext token.
func (s *Service) issue(ctx context.Context, q db.DBTX, agentID, spaceID int64) (string, error) {
	token, err := NewToken()
	if err != nil {
		return "", err
	}
	if err := insertToken(ctx, q, Hash(token), agentID, spaceID, s.stamp()); err != nil {
		return "", err
	}
	return token, nil
}

// RevokeSpaceTokens deletes every token of agentID bound to spaceID.
func (s *Service) RevokeSpaceTokens(ctx context.Context, q db.DBTX, agentID, spaceID int64) error {
	return deleteSpaceTokens(ctx, q, agentID, spaceID)
}

func (s *Service) stamp() string {
	return db.Timestamp(s.now())
}
