package invites

import (
	"context"
	"database/sql"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/domain"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

type admission interface {
	AuthorizeInvite(ctx context.Context, tx db.DBTX, p auth.Principal, space, name string) (int64, error)
	AdmitMember(ctx context.Context, tx db.DBTX, spaceID int64, name string) (space, token string, err error)
}

// TTL is how long an invite code stays redeemable.
const TTL = 10 * time.Minute

const invalidCode = "invalid or expired invite code"

// Invite is a freshly created invite code.
type Invite struct {
	Name, Space, Code string
	ExpiresAt         time.Time
}

// Admission is the result of redeeming an invite code and the 200 body of POST /v1/invites/accept;
// Task is empty, and omitted from JSON, when the invite had none.
type Admission struct {
	Name  string `json:"name"`
	Space string `json:"space"`
	Token string `json:"token"`
	Task  string `json:"task,omitempty"`
}

// Service creates and redeems invite codes.
type Service struct {
	db    *sql.DB
	admit admission
	now   func() time.Time
}

// NewService returns an invites Service admitting members through a.
func NewService(conn *sql.DB, a admission, now func() time.Time) *Service {
	return &Service{db: conn, admit: a, now: now}
}

// Create issues a single-use code that admits member name into space; leaders of the space only.
// A non-nil task is the member's first prompt, returned once when the code is accepted.
func (s *Service) Create(ctx context.Context, p auth.Principal, space, name string, task *string) (Invite, error) {
	r := row{name: name}
	if task != nil {
		if err := domain.ValidBody(*task); err != nil {
			return Invite{}, err
		}
		r.task = sql.NullString{String: *task, Valid: true}
	}
	now := s.now()
	inv := Invite{Name: name, Space: space, ExpiresAt: now.Add(TTL)}
	r.expiresAt = inv.ExpiresAt.Unix()
	err := db.InTx(ctx, s.db, func(tx *sql.Tx) error {
		var err error
		inv.Code, err = s.store(ctx, tx, p, space, r, now)
		return err
	})
	if err != nil {
		return Invite{}, err
	}
	return inv, nil
}

// store purges expired invites, authorizes p and stores r under a new code, which it returns.
func (s *Service) store(ctx context.Context, tx db.DBTX, p auth.Principal, space string, r row, now time.Time) (string, error) {
	if err := deleteExpired(ctx, tx, now.Unix()); err != nil {
		return "", err
	}
	var err error
	if r.spaceID, err = s.admit.AuthorizeInvite(ctx, tx, p, space, r.name); err != nil {
		return "", err
	}
	code, err := newCode()
	if err != nil {
		return "", err
	}
	r.hash = auth.Hash(normalize(code))
	return code, insertInvite(ctx, tx, r)
}

// Accept redeems code once for a member token. A failed admission keeps the code.
func (s *Service) Accept(ctx context.Context, code string) (Admission, error) {
	norm := normalize(code)
	if len(norm) != codeLen {
		return Admission{}, apperr.New(apperr.Unauthenticated, invalidCode)
	}
	var a Admission
	err := db.InTx(ctx, s.db, func(tx *sql.Tx) error {
		r, found, err := takeInvite(ctx, tx, auth.Hash(norm))
		if err != nil {
			return err
		}
		if !found || s.now().Unix() >= r.expiresAt {
			return apperr.New(apperr.Unauthenticated, invalidCode)
		}
		a.Name, a.Task = r.name, r.task.String
		a.Space, a.Token, err = s.admit.AdmitMember(ctx, tx, r.spaceID, r.name)
		return err
	})
	if err != nil {
		return Admission{}, err
	}
	return a, nil
}
