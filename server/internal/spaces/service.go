package spaces

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

type tokenMinter interface {
	MintMember(ctx context.Context, tx auth.DBTX, name string, spaceID int64) (agentID int64, token string, err error)
}

type inboxWriter interface {
	Insert(ctx context.Context, tx auth.DBTX, es []inbox.Entry) ([]inbox.Entry, error)
	Publish(es []inbox.Entry)
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
	db     *sql.DB
	r      *Resolver
	tokens tokenMinter
	ib     inboxWriter
	now    func() time.Time
	hooks  []DepartureHook
}

// NewService returns a spaces Service; hooks run when a participant departs.
func NewService(db *sql.DB, r *Resolver, tokens tokenMinter, ib inboxWriter, now func() time.Time, hooks ...DepartureHook) *Service {
	return &Service{db: db, r: r, tokens: tokens, ib: ib, now: now, hooks: hooks}
}

// inTx runs fn in one transaction, inserts its entries, commits and publishes.
func (s *Service) inTx(ctx context.Context, fn func(tx *sql.Tx) ([]inbox.Entry, error)) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	entries, err := fn(tx)
	if err != nil {
		return err
	}
	inserted, err := s.ib.Insert(ctx, tx, entries)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing: %w", err)
	}
	s.ib.Publish(inserted)
	return nil
}

func (s *Service) stamp() string { return s.now().UTC().Format(time.RFC3339Nano) }

// List returns the spaces visible to p: all for leaders, its own for members.
func (s *Service) List(ctx context.Context, p auth.Principal) ([]SpaceInfo, error) {
	if p.Kind == auth.Admin {
		return nil, apperr.New(apperr.Forbidden, "admin token cannot act in spaces; mint a leader with bus_mint_leader")
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

func requireLeaderPrincipal(p auth.Principal, action string) error {
	if p.Kind != auth.Leader {
		return apperr.New(apperr.Forbidden, "only leaders may %s", action)
	}
	return nil
}

func requireLeaderRole(m Membership, action string) error {
	if m.Role != RoleLeader {
		return apperr.New(apperr.Forbidden, "only leaders may %s", action)
	}
	return nil
}

// Create makes a new space led by p, with the open topic all.
func (s *Service) Create(ctx context.Context, p auth.Principal, name string) error {
	if err := requireLeaderPrincipal(p, "create spaces"); err != nil {
		return err
	}
	if err := auth.ValidName("space", name); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
		exists, err := spaceExists(ctx, tx, name)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, apperr.New(apperr.Conflict, "space %s already exists", name)
		}
		ts := s.stamp()
		id, err := insertSpace(ctx, tx, name, p.AgentID, ts)
		if err != nil {
			return nil, err
		}
		if err := insertParticipant(ctx, tx, id, p.AgentID, RoleLeader, ts); err != nil {
			return nil, err
		}
		return nil, insertAllTopic(ctx, tx, id, p.AgentID, ts)
	})
}

// Join adds leader p to an existing space as a leader.
func (s *Service) Join(ctx context.Context, p auth.Principal, name string) error {
	if err := requireLeaderPrincipal(p, "join spaces"); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
		_, err := s.r.ResolveTx(ctx, tx, p, name)
		if err == nil {
			return nil, apperr.New(apperr.Conflict, "already a participant of %s", name)
		}
		if kind, _ := apperr.KindOf(err); kind != apperr.Forbidden {
			return nil, err
		}
		var id int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM spaces WHERE name = ?`, name).Scan(&id); err != nil {
			return nil, fmt.Errorf("looking up space %s: %w", name, err)
		}
		return nil, insertParticipant(ctx, tx, id, p.AgentID, RoleLeader, s.stamp())
	})
}

// Delete removes a space; only its sole remaining leader may do so.
func (s *Service) Delete(ctx context.Context, p auth.Principal, name string) error {
	return s.inTx(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
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
	err := s.inTx(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
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
	return s.inTx(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
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
	return s.inTx(ctx, func(tx *sql.Tx) ([]inbox.Entry, error) {
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

// removeParticipant runs hooks, then deletes inbox rows, space tokens and the participant.
func (s *Service) removeParticipant(ctx context.Context, tx *sql.Tx, m Membership) ([]inbox.Entry, error) {
	var entries []inbox.Entry
	for _, h := range s.hooks {
		es, err := h.OnDeparture(ctx, tx, m)
		if err != nil {
			return nil, fmt.Errorf("running departure hook: %w", err)
		}
		entries = append(entries, es...)
	}
	if err := deleteParticipantData(ctx, tx, m.SpaceID, m.AgentID); err != nil {
		return nil, err
	}
	return entries, nil
}

// Participants lists the participants of space, leaders first.
func (s *Service) Participants(ctx context.Context, p auth.Principal, space string) ([]Participant, error) {
	m, err := s.r.Resolve(ctx, p, space)
	if err != nil {
		return nil, err
	}
	return listParticipants(ctx, s.db, m.SpaceID)
}
