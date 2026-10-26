package invites

import (
	"context"
	"database/sql"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/domain"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

type admission interface {
	AuthorizeInvite(ctx context.Context, tx db.DBTX, p auth.Principal, space, name string) (int64, error)
	AdmitMember(ctx context.Context, tx db.DBTX, spaceID int64, name string) (agentID int64, space, token string, err error)
}

// deliverer stores inbox entries inside a transaction and publishes them after commit.
type deliverer interface {
	Deliver(ctx context.Context, fn func(tx *sql.Tx) ([]inbox.Entry, error)) error
}

// TTL is how long an invite code stays redeemable.
const TTL = 10 * time.Minute

const invalidCode = "invalid or expired invite code"

// Invite is a freshly created invite code.
type Invite struct {
	Name, Space, Code string
	ExpiresAt         time.Time
}

// Admission is the result of redeeming an invite code and the 200 body of POST /v1/invites/accept.
type Admission struct {
	Name  string `json:"name"`
	Space string `json:"space"`
	Token string `json:"token"`
}

// Service creates and redeems invite codes.
type Service struct {
	db    *sql.DB
	admit admission
	ib    deliverer
	now   func() time.Time
}

// NewService returns an invites Service admitting members through a and delivering their
// tasks through ib.
func NewService(conn *sql.DB, a admission, ib deliverer, now func() time.Time) *Service {
	return &Service{db: conn, admit: a, ib: ib, now: now}
}

// Create issues a single-use code that admits member name into space; leaders of the space
// and admins. A non-nil task is the member's first prompt, delivered as a direct message
// from p when the code is accepted.
func (s *Service) Create(ctx context.Context, p auth.Principal, space, name string, task *string) (Invite, error) {
	r := row{name: name, invitedBy: p.AgentName}
	if task != nil {
		if err := domain.ValidText("task", *task); err != nil {
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

// Accept redeems code once for a member token and, in the same transaction, stores the
// invite's task as a direct message from the inviter. A failed admission keeps the code
// and delivers nothing.
func (s *Service) Accept(ctx context.Context, code string) (Admission, error) {
	norm := normalize(code)
	if len(norm) != codeLen {
		return Admission{}, apperr.New(apperr.Unauthenticated, invalidCode)
	}
	var a Admission
	err := s.ib.Deliver(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
		r, found, err := takeInvite(ctx, tx, auth.Hash(norm))
		if err != nil {
			return nil, err
		}
		if !found || s.now().Unix() >= r.expiresAt {
			return nil, apperr.New(apperr.Unauthenticated, invalidCode)
		}
		agentID, space, token, err := s.admit.AdmitMember(ctx, tx, r.spaceID, r.name)
		if err != nil {
			return nil, err
		}
		a = Admission{Name: r.name, Space: space, Token: token}
		return taskEntry(r, agentID, space), nil
	})
	if err != nil {
		return Admission{}, err
	}
	return a, nil
}

// taskEntry is the invite's task as a direct message from the inviter to member agentID,
// or nothing when the invite has no task.
func taskEntry(r row, agentID int64, space string) []inbox.Entry {
	if !r.task.Valid {
		return nil
	}
	return []inbox.Entry{{AgentID: agentID, SpaceID: r.spaceID, Space: space,
		Type: inbox.DirectMessage, From: r.invitedBy, Body: r.task.String}}
}
