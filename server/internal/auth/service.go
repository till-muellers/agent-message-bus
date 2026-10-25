package auth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"fmt"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

// Service authenticates bearer tokens and mints leader and member tokens.
type Service struct {
	db        *sql.DB
	adminHash string
	now       func() time.Time
}

// NewService returns a Service using adminToken as the admin credential.
func NewService(db *sql.DB, adminToken string, now func() time.Time) *Service {
	return &Service{db: db, adminHash: Hash(adminToken), now: now}
}

// Authenticate resolves token to a Principal.
func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	h := Hash(token)
	if subtle.ConstantTimeCompare([]byte(h), []byte(s.adminHash)) == 1 {
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
	if err := ValidName("name", name); err != nil {
		return "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("beginning tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	id, err := s.ensureAgent(ctx, tx, name, Leader)
	if err != nil {
		return "", err
	}
	token, err := s.issue(ctx, tx, id, 0)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("committing leader token: %w", err)
	}
	return token, nil
}

// MintMember creates (or reuses) member agent name and issues a token bound to spaceID.
func (s *Service) MintMember(ctx context.Context, tx DBTX, name string, spaceID int64) (agentID int64, token string, err error) {
	if err := ValidName("name", name); err != nil {
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
func (s *Service) ensureAgent(ctx context.Context, q DBTX, name string, kind Kind) (int64, error) {
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
func (s *Service) issue(ctx context.Context, q DBTX, agentID, spaceID int64) (string, error) {
	token, err := NewToken()
	if err != nil {
		return "", err
	}
	if err := insertToken(ctx, q, Hash(token), agentID, spaceID, s.stamp()); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) stamp() string {
	return s.now().UTC().Format(time.RFC3339Nano)
}
