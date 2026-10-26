package auth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/domain"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// Service authenticates bearer tokens and mints admin, leader and member tokens.
type Service struct {
	db          *sql.DB
	adminHash   atomic.Pointer[string]
	bootstrapID atomic.Int64 // agent id of the bootstrap admin, set by Bootstrap
	now         func() time.Time
	log         *slog.Logger // audit lines ("admin minted")
}

// NewService returns a Service using adminToken as the admin credential; audit lines go
// to slog.Default().
func NewService(db *sql.DB, adminToken string, now func() time.Time) *Service {
	s := &Service{db: db, now: now, log: slog.Default()}
	h := Hash(adminToken)
	s.adminHash.Store(&h)
	return s
}

// Authenticate resolves token to a Principal.
func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	h := Hash(token)
	if subtle.ConstantTimeCompare([]byte(h), []byte(*s.adminHash.Load())) == 1 {
		return s.bootstrapPrincipal(), nil
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

// MintLeader creates (or reuses) leader agent name and returns a new token. Admins only.
func (s *Service) MintLeader(ctx context.Context, p Principal, name string) (string, error) {
	if err := RequireAdmin(p); err != nil {
		return "", err
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
		token, err = s.issue(ctx, tx, TokenPrefix, id, 0)
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
	token, err = s.issue(ctx, tx, TokenPrefix, agentID, spaceID)
	if err != nil {
		return 0, "", err
	}
	return agentID, token, nil
}

// CheckMemberName validates name as a member agent name without creating it.
func (s *Service) CheckMemberName(ctx context.Context, q db.DBTX, name string) error {
	if err := domain.ValidName("name", name); err != nil {
		return err
	}
	_, _, err := s.agentOfKind(ctx, q, name, Member)
	return err
}

// ensureAgent returns the id of agent name with kind, creating it if missing.
func (s *Service) ensureAgent(ctx context.Context, q db.DBTX, name string, kind Kind) (int64, error) {
	id, found, err := s.agentOfKind(ctx, q, name, kind)
	if err != nil || found {
		return id, err
	}
	return insertAgent(ctx, q, name, kind, s.stamp())
}

// agentOfKind looks up agent name; an existing agent of another kind is a Conflict.
func (s *Service) agentOfKind(ctx context.Context, q db.DBTX, name string, kind Kind) (id int64, found bool, err error) {
	id, existing, found, err := findAgent(ctx, q, name)
	if err != nil || !found {
		return 0, false, err
	}
	if existing != kind {
		return 0, false, apperr.New(apperr.Conflict, "agent %s is %s %s agent", name, article(existing), existing)
	}
	return id, true, nil
}

// article returns the indefinite article for kind.
func article(kind Kind) string {
	if kind == Admin {
		return "an"
	}
	return "a"
}

// issue stores a fresh token's hash and returns the plaintext token, which starts with prefix.
func (s *Service) issue(ctx context.Context, q db.DBTX, prefix string, agentID, spaceID int64) (string, error) {
	token, err := NewToken(prefix)
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
