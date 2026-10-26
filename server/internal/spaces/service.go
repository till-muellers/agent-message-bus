package spaces

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/domain"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// memberAccounts mints, vets and revokes the member agents that spaces admit.
type memberAccounts interface {
	MintMember(ctx context.Context, tx db.DBTX, name string, spaceID int64) (agentID int64, token string, err error)
	RevokeSpaceTokens(ctx context.Context, q db.DBTX, agentID, spaceID int64) error
	CheckMemberName(ctx context.Context, q db.DBTX, name string) error
}

type inboxStore interface {
	Deliver(ctx context.Context, fn func(tx *sql.Tx) ([]inbox.Entry, error)) error
	DeleteSpaceEntries(ctx context.Context, q db.DBTX, agentID, spaceID int64) error
}

// SpaceInfo summarises a space for listing. Admins are the joined admins; Members
// counts role member only.
type SpaceInfo struct {
	Name    string   `json:"name"`
	Leaders []string `json:"leaders"`
	Admins  []string `json:"admins"`
	Members int      `json:"members"`
	Joined  bool     `json:"joined"`
}

// Participant is one agent in a space.
type Participant struct {
	Name string `json:"name"`
	Role Role   `json:"role"`
}

// Service manages spaces and their participants.
type Service struct {
	db       *sql.DB
	r        *Resolver
	accounts memberAccounts
	ib       inboxStore
	now      func() time.Time
	hooks    Hooks
}

// NewService returns a spaces Service; departure hooks run when a participant departs,
// creation hooks when a space is created and revocation hooks when a name is revoked.
func NewService(conn *sql.DB, r *Resolver, a memberAccounts, ib inboxStore, now func() time.Time, hooks Hooks) *Service {
	return &Service{db: conn, r: r, accounts: a, ib: ib, now: now, hooks: hooks}
}

func (s *Service) stamp() string { return db.Timestamp(s.now()) }

// List returns the spaces visible to p: all for leaders and admins, its own for members.
func (s *Service) List(ctx context.Context, p auth.Principal) ([]SpaceInfo, error) {
	rows, err := listSpaces(ctx, s.db, p.SpaceID)
	if err != nil {
		return nil, err
	}
	out := make([]SpaceInfo, 0, len(rows))
	for _, row := range rows {
		ps, err := listParticipants(ctx, s.db, row.id)
		if err != nil {
			return nil, err
		}
		out = append(out, summarize(row.name, ps, p.AgentName))
	}
	return out, nil
}

func summarize(name string, ps []Participant, caller string) SpaceInfo {
	info := SpaceInfo{Name: name, Leaders: []string{}, Admins: []string{}}
	for _, pt := range ps {
		switch pt.Role {
		case RoleAdmin:
			info.Admins = append(info.Admins, pt.Name)
		case RoleLeader:
			info.Leaders = append(info.Leaders, pt.Name)
		default:
			info.Members++
		}
		if pt.Name == caller {
			info.Joined = true
		}
	}
	return info
}

func requireLeaderRole(m Membership, action string) error {
	if !m.Leads() {
		return apperr.New(apperr.Forbidden, "only leaders may %s", action)
	}
	return nil
}

// Create makes a new space with p as its first participant (role leader, or admin for
// an admin) and runs the creation hooks.
func (s *Service) Create(ctx context.Context, p auth.Principal, name string) error {
	if err := requireLeaderPrincipal(p, "create spaces"); err != nil {
		return err
	}
	if err := domain.ValidName("space", name); err != nil {
		return err
	}
	return s.ib.Deliver(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
		return nil, s.createInTx(ctx, tx, p, name)
	})
}

func (s *Service) createInTx(ctx context.Context, tx *sql.Tx, p auth.Principal, name string) error {
	exists, err := spaceExists(ctx, tx, name)
	if err != nil {
		return err
	}
	if exists {
		return apperr.New(apperr.Conflict, "space %s already exists", name)
	}
	ts := s.stamp()
	id, err := insertSpace(ctx, tx, name, p.AgentID, ts)
	if err != nil {
		return err
	}
	m := Membership{SpaceID: id, SpaceName: name, AgentID: p.AgentID, AgentName: p.AgentName,
		Role: joinRole(p), Admin: p.Kind == auth.Admin}
	if err := insertParticipant(ctx, tx, id, p.AgentID, m.Role, ts); err != nil {
		return err
	}
	for _, h := range s.hooks.Creation {
		if err := h.OnSpaceCreated(ctx, tx, m); err != nil {
			return fmt.Errorf("running creation hook: %w", err)
		}
	}
	return nil
}

// Join adds leader or admin p to an existing space, with role leader or admin.
func (s *Service) Join(ctx context.Context, p auth.Principal, name string) error {
	if err := requireLeaderPrincipal(p, "join spaces"); err != nil {
		return err
	}
	return s.ib.Deliver(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
		id, found, err := spaceIDByName(ctx, tx, name)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, apperr.New(apperr.NotFound, "space %s not found", name)
		}
		if _, joined, err := participantRole(ctx, tx, id, p.AgentID); err != nil || joined {
			if err == nil {
				err = apperr.New(apperr.Conflict, "already a participant of %s", name)
			}
			return nil, err
		}
		return nil, insertParticipant(ctx, tx, id, p.AgentID, joinRole(p), s.stamp())
	})
}

// joinRole is the participant role of leader or admin p in a space it creates or joins.
func joinRole(p auth.Principal) Role {
	if p.Kind == auth.Admin {
		return RoleAdmin
	}
	return RoleLeader
}

// Delete removes a space. Admins may delete any space; a leader only once no other
// leader remains.
func (s *Service) Delete(ctx context.Context, p auth.Principal, name string) error {
	return s.ib.Deliver(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
		m, err := s.r.ResolveTx(ctx, tx, p, name)
		if err != nil {
			return nil, err
		}
		if err := requireLeaderRole(m, "delete spaces"); err != nil {
			return nil, err
		}
		if m.Admin {
			return nil, deleteSpace(ctx, tx, m.SpaceID)
		}
		n, err := countOtherLeaders(ctx, tx, m.SpaceID, m.AgentID)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			return nil, apperr.New(apperr.Conflict, "space %s has other leaders; they must leave first", name)
		}
		return nil, deleteSpace(ctx, tx, m.SpaceID)
	})
}

// Leave removes p from space; the last leader cannot leave, and an admin that has not
// joined has nothing to leave.
func (s *Service) Leave(ctx context.Context, p auth.Principal, space string) error {
	return s.ib.Deliver(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
		m, err := s.r.ResolveTx(ctx, tx, p, space)
		if err != nil {
			return nil, err
		}
		if !m.Joined() {
			return nil, apperr.New(apperr.Forbidden, "not a participant of space %s", space)
		}
		if m.Role == RoleLeader {
			n, err := countOtherLeaders(ctx, tx, m.SpaceID, m.AgentID)
			if err != nil {
				return nil, err
			}
			if n == 0 {
				return nil, apperr.New(apperr.Conflict, "last leader cannot leave; delete the space instead")
			}
		}
		return s.removeParticipant(ctx, tx, m)
	})
}

// removeParticipant runs departure hooks, then deletes inbox rows, space tokens and the participant.
func (s *Service) removeParticipant(ctx context.Context, tx *sql.Tx, m Membership) ([]inbox.Entry, error) {
	var entries []inbox.Entry
	for _, h := range s.hooks.Departure {
		es, err := h.OnDeparture(ctx, tx, m)
		if err != nil {
			return nil, fmt.Errorf("running departure hook: %w", err)
		}
		entries = append(entries, es...)
	}
	if err := s.ib.DeleteSpaceEntries(ctx, tx, m.AgentID, m.SpaceID); err != nil {
		return nil, err
	}
	if err := s.accounts.RevokeSpaceTokens(ctx, tx, m.AgentID, m.SpaceID); err != nil {
		return nil, err
	}
	return entries, deleteParticipant(ctx, tx, m.SpaceID, m.AgentID)
}

// Participants lists the participants of space, leaders first.
func (s *Service) Participants(ctx context.Context, p auth.Principal, space string) ([]Participant, error) {
	m, err := s.r.Resolve(ctx, p, space)
	if err != nil {
		return nil, err
	}
	return listParticipants(ctx, s.db, m.SpaceID)
}
