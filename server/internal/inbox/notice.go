package inbox

import (
	"context"
	"database/sql"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// Notice says what waits in one mailbox scope, never what it says: no bodies.
type Notice struct {
	Waiting  int      `json:"waiting"`
	Critical int      `json:"critical"`
	Spaces   []string `json:"spaces"`
}

// isEdge reports whether e, about to be stored through q, is an edge: its recipient's
// mailbox scope holds no entry yet, or e is critical (PO-7 re-notify). The scope is
// (agent, space) for member agents and (agent) for leaders and admins. Entries stored
// earlier in the same tx count, so a tx yields at most one non-critical edge per scope.
func isEdge(ctx context.Context, q db.DBTX, e Entry) (bool, error) {
	if e.Critical {
		return true, nil
	}
	occupied, err := scopeOccupied(ctx, q, e.AgentID, e.SpaceID)
	if err != nil {
		return false, err
	}
	return !occupied, nil
}

// TakeNotice reports whether p's mailbox scope holds an edge nobody noticed yet and,
// if so, marks every such edge noticed and returns what waits in the scope.
func (s *Service) TakeNotice(ctx context.Context, p auth.Principal) (Notice, bool, error) {
	var n Notice
	var due bool
	err := db.InTx(ctx, s.db, func(tx *sql.Tx) error {
		marked, err := markNoticed(ctx, tx, p.AgentID, spaceFilter(p))
		if err != nil || marked == 0 {
			return err
		}
		due = true
		n, err = waitingIn(ctx, tx, p.AgentID, spaceFilter(p))
		return err
	})
	if err != nil {
		return Notice{}, false, err
	}
	return n, due, nil
}
