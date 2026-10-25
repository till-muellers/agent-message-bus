package blackboards

import (
	"context"
	"database/sql"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/domain"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// BoardInfo summarizes a blackboard in a space.
type BoardInfo struct {
	Name    string `json:"name"`
	Owner   string `json:"owner"`
	Threads int    `json:"threads"`
}

// List returns the space's blackboards ordered by name.
func (s *Service) List(ctx context.Context, p auth.Principal, space string) ([]BoardInfo, error) {
	var out []BoardInfo
	err := s.inSpace(ctx, p, space, func(tx *sql.Tx, m spaces.Membership) ([]inbox.Entry, error) {
		var err error
		out, err = listBoards(ctx, tx, m.SpaceID)
		return nil, err
	})
	return out, err
}

// Create adds a blackboard owned by the caller; any participant may create one.
func (s *Service) Create(ctx context.Context, p auth.Principal, space, name string) error {
	if err := domain.ValidName("blackboard", name); err != nil {
		return err
	}
	return s.inSpace(ctx, p, space, func(tx *sql.Tx, m spaces.Membership) ([]inbox.Entry, error) {
		_, found, err := findBoard(ctx, tx, m.SpaceID, name)
		if err != nil {
			return nil, err
		}
		if found {
			return nil, apperr.New(apperr.Conflict, "blackboard %s already exists", name)
		}
		return nil, insertBoard(ctx, tx, m.SpaceID, name, m.AgentID, s.stamp())
	})
}

// Delete removes a blackboard (owner or leader), notifying its thread subscribers except the caller.
func (s *Service) Delete(ctx context.Context, p auth.Principal, space, name string) error {
	return s.inSpace(ctx, p, space, func(tx *sql.Tx, m spaces.Membership) ([]inbox.Entry, error) {
		b, err := mustFindBoard(ctx, tx, m.SpaceID, name)
		if err != nil {
			return nil, err
		}
		if b.ownerID != m.AgentID && m.Role != spaces.RoleLeader {
			return nil, apperr.New(apperr.Forbidden, "only the owner or a leader may delete blackboard %s", name)
		}
		return removeBoard(ctx, tx, m, b)
	})
}

// removeBoard notifies the board's thread subscribers except m and deletes the board.
func removeBoard(ctx context.Context, tx db.DBTX, m spaces.Membership, b board) ([]inbox.Entry, error) {
	ids, err := boardSubscribers(ctx, tx, b.id, m.AgentID)
	if err != nil {
		return nil, err
	}
	if err := deleteBoard(ctx, tx, b.id); err != nil {
		return nil, err
	}
	return notify(m, ids, inbox.BlackboardRemoved, inbox.Entry{Blackboard: b.name}), nil
}

func mustFindBoard(ctx context.Context, q db.DBTX, spaceID int64, name string) (board, error) {
	b, found, err := findBoard(ctx, q, spaceID, name)
	if err != nil {
		return board{}, err
	}
	if !found {
		return board{}, apperr.New(apperr.NotFound, "blackboard %s not found", name)
	}
	return b, nil
}
