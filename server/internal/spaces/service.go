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

type tokenStore interface {
	MintMember(ctx context.Context, tx db.DBTX, name string, spaceID int64) (agentID int64, token string, err error)
	RevokeSpaceTokens(ctx context.Context, q db.DBTX, agentID, spaceID int64) error
}

type inboxStore interface {
	Deliver(ctx context.Context, fn func(tx *sql.Tx) ([]inbox.Entry, error)) error
	DeleteSpaceEntries(ctx context.Context, q db.DBTX, agentID, spaceID int64) error
}

// SpaceInfo summarises a space for listing.
type SpaceInfo struct {
	Name    string   `json:"name"`
	Leaders []string `json:"leaders"`
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
	db        *sql.DB
	r         *Resolver
	tokens    tokenStore
	ib        inboxStore
	now       func() time.Time
	departure []DepartureHook
	creation  []CreationHook
}

// NewService returns a spaces Service; departure hooks run when a participant
// departs, creation hooks when a space is created.
func NewService(conn *sql.DB, r *Resolver, a tokenStore, ib inboxStore, now func() time.Time,
	departure []DepartureHook, creation []CreationHook) *Service {
	return &Service{db: conn, r: r, tokens: a, ib: ib, now: now, departure: departure, creation: creation}
}

func (s *Service) stamp() string { return db.Timestamp(s.now()) }

// List returns the spaces visible to p: all for leaders, its own for members.
func (s *Service) List(ctx context.Context, p auth.Principal) ([]SpaceInfo, error) {
	if err := requireAgent(p); err != nil {
		return nil, err
	}
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
	info := SpaceInfo{Name: name, Leaders: []string{}}
	for _, pt := range ps {
		if pt.Role == RoleLeader {
			info.Leaders = append(info.Leaders, pt.Name)
		} else {
			info.Members++
		}
		if pt.Name == caller {
			info.Joined = true
		}
	}
	return info
}

func requireLeaderRole(m Membership, action string) error {
	if m.Role != RoleLeader {
		return apperr.New(apperr.Forbidden, "only leaders may %s", action)
	}
	return nil
}

// Create makes a new space led by p and runs the creation hooks.
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
	if err := insertParticipant(ctx, tx, id, p.AgentID, RoleLeader, ts); err != nil {
		return err
	}
	m := Membership{SpaceID: id, SpaceName: name, AgentID: p.AgentID, AgentName: p.AgentName, Role: RoleLeader}
	for _, h := range s.creation {
		if err := h.OnSpaceCreated(ctx, tx, m); err != nil {
			return fmt.Errorf("running creation hook: %w", err)
		}
	}
	return nil
}

// Join adds leader p to an existing space as a leader.
func (s *Service) Join(ctx context.Context, p auth.Principal, name string) error {
	if err := requireLeaderPrincipal(p, "join spaces"); err != nil {
		return err
	}
	return s.ib.Deliver(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
		_, err := s.r.ResolveTx(ctx, tx, p, name)
		if err == nil {
			return nil, apperr.New(apperr.Conflict, "already a participant of %s", name)
		}
		if kind, _ := apperr.KindOf(err); kind != apperr.Forbidden {
			return nil, err
		}
		id, _, err := spaceIDByName(ctx, tx, name)
		if err != nil {
			return nil, err
		}
		return nil, insertParticipant(ctx, tx, id, p.AgentID, RoleLeader, s.stamp())
	})
}

// Delete removes a space; only its sole remaining leader may do so.
func (s *Service) Delete(ctx context.Context, p auth.Principal, name string) error {
	return s.ib.Deliver(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
		m, err := s.r.ResolveTx(ctx, tx, p, name)
		if err != nil {
			return nil, err
		}
		if err := requireLeaderRole(m, "delete spaces"); err != nil {
			return nil, err
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

// Invite mints a member token for name in space, adding the participant if new.
func (s *Service) Invite(ctx context.Context, p auth.Principal, space, name string) (string, error) {
	var token string
	err := s.ib.Deliver(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
		m, err := s.r.ResolveTx(ctx, tx, p, space)
		if err != nil {
			return nil, err
		}
		if err := requireLeaderRole(m, "invite members"); err != nil {
			return nil, err
		}
		agentID, tok, err := s.tokens.MintMember(ctx, tx, name, m.SpaceID)
		if err != nil {
			return nil, err
		}
		token = tok
		_, found, err := participantRole(ctx, tx, m.SpaceID, agentID)
		if err != nil || found {
			return nil, err
		}
		return nil, insertParticipant(ctx, tx, m.SpaceID, agentID, RoleMember, s.stamp())
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// Revoke removes member name from space.
func (s *Service) Revoke(ctx context.Context, p auth.Principal, space, name string) error {
	return s.ib.Deliver(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
		m, err := s.r.ResolveTx(ctx, tx, p, space)
		if err != nil {
			return nil, err
		}
		if err := requireLeaderRole(m, "revoke members"); err != nil {
			return nil, err
		}
		id, role, found, err := participantByName(ctx, tx, m.SpaceID, name)
		if err != nil {
			return nil, err
		}
		if !found || role != RoleMember {
			return nil, apperr.New(apperr.NotFound, "member %s not in space %s", name, space)
		}
		target := Membership{SpaceID: m.SpaceID, SpaceName: space, AgentID: id, AgentName: name, Role: role}
		return s.removeParticipant(ctx, tx, target)
	})
}

// Leave removes p from space; the last leader cannot leave.
func (s *Service) Leave(ctx context.Context, p auth.Principal, space string) error {
	return s.ib.Deliver(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
		m, err := s.r.ResolveTx(ctx, tx, p, space)
		if err != nil {
			return nil, err
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
	for _, h := range s.departure {
		es, err := h.OnDeparture(ctx, tx, m)
		if err != nil {
			return nil, fmt.Errorf("running departure hook: %w", err)
		}
		entries = append(entries, es...)
	}
	if err := s.ib.DeleteSpaceEntries(ctx, tx, m.AgentID, m.SpaceID); err != nil {
		return nil, err
	}
	if err := s.tokens.RevokeSpaceTokens(ctx, tx, m.AgentID, m.SpaceID); err != nil {
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
