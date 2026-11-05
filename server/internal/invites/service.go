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
	AuthorizeInvites(ctx context.Context, q db.DBTX, p auth.Principal, space string) (int64, error)
}

// stager stores inbox entries in the caller's transaction and publishes them after commit.
type stager interface {
	Stage(ctx context.Context, tx db.DBTX, es []inbox.Entry) (inbox.Staged, error)
	Publish(st inbox.Staged) []inbox.Receipt
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

// Pending is an open invite as the web GUI lists it: its opaque id, never the code.
type Pending struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	InvitedBy string `json:"invitedBy"`
	ExpiresAt string `json:"expiresAt"` // RFC3339 UTC
}

// Service creates, redeems and cancels invite codes; it is the only writer of the invites table.
type Service struct {
	db    *sql.DB
	admit admission
	ib    stager
	now   func() time.Time
}

// NewService returns an invites Service admitting members through a and delivering their
// tasks through ib.
func NewService(conn *sql.DB, a admission, ib stager, now func() time.Time) *Service {
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

// store purges expired invites, authorizes p and stores r under a new code and id; it
// returns the code.
func (s *Service) store(ctx context.Context, tx db.DBTX, p auth.Principal, space string, r row, now time.Time) (string, error) {
	if err := deleteExpired(ctx, tx, now.Unix()); err != nil {
		return "", err
	}
	var err error
	if r.spaceID, err = s.admit.AuthorizeInvite(ctx, tx, p, space, r.name); err != nil {
		return "", err
	}
	if r.id, err = newID(); err != nil {
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
	_, err := inbox.InTx(ctx, s.db, s.ib, func(tx db.DBTX) ([]inbox.Entry, error) {
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
	from := inbox.Sender{SpaceID: r.spaceID, Space: space, Name: r.invitedBy}
	return from.Notify([]int64{agentID}, inbox.Entry{Type: inbox.DirectMessage, Body: r.task.String})
}

// ListPending returns the open invites of space, soonest expiry first; redeemed, expired and
// revoked codes are gone. Leaders of the space and admins.
func (s *Service) ListPending(ctx context.Context, p auth.Principal, space string) ([]Pending, error) {
	spaceID, err := s.admit.AuthorizeInvites(ctx, s.db, p, space)
	if err != nil {
		return nil, err
	}
	return s.ListOpen(ctx, spaceID)
}

// ListOpen is ListPending for spaceID without the gate: the caller has authorized it with
// spaces.Membership.MaySeeInvites.
func (s *Service) ListOpen(ctx context.Context, spaceID int64) ([]Pending, error) {
	return selectPending(ctx, s.db, spaceID, s.now().Unix())
}

// CountOpen counts the open invites of each of spaceIDs in one read; a space without open
// invites is absent. No gate: the caller asks spaces.Membership.MaySeeInvites per space.
func (s *Service) CountOpen(ctx context.Context, spaceIDs []int64) (map[int64]int, error) {
	return countOpen(ctx, s.db, spaceIDs, s.now().Unix())
}

// Cancel deletes the open invite id of space and returns its invited name; cancelled is
// false for an unknown, redeemed, expired or already cancelled id and for an id of another
// space. Leaders of the space and admins.
func (s *Service) Cancel(ctx context.Context, p auth.Principal, space, id string) (name string, cancelled bool, err error) {
	err = db.InTx(ctx, s.db, func(tx *sql.Tx) error {
		spaceID, authErr := s.admit.AuthorizeInvites(ctx, tx, p, space)
		if authErr != nil {
			return authErr
		}
		if purgeErr := deleteExpired(ctx, tx, s.now().Unix()); purgeErr != nil {
			return purgeErr
		}
		name, cancelled, err = deleteByID(ctx, tx, spaceID, id)
		return err
	})
	if err != nil {
		return "", false, err
	}
	return name, cancelled, nil
}
