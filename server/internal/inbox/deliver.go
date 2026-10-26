package inbox

import (
	"context"
	"database/sql"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// Deliver runs fn in a transaction, inserts the entries it returns in the same
// transaction and publishes them once it commits. fn MUST use only tx.
func (s *Service) Deliver(ctx context.Context, fn func(tx *sql.Tx) ([]Entry, error)) error {
	_, err := s.DeliverReceipts(ctx, fn)
	return err
}

// DeliverReceipts is Deliver that also returns one receipt per distinct recipient,
// sorted by name: live when the recipient is connected right after the commit, else
// queued. No entries → [].
func (s *Service) DeliverReceipts(ctx context.Context, fn func(tx *sql.Tx) ([]Entry, error)) ([]Receipt, error) {
	var stored []Entry
	var names []recipient
	err := db.InTx(ctx, s.db, func(tx *sql.Tx) error {
		es, err := fn(tx)
		if err != nil {
			return err
		}
		if stored, err = s.insert(ctx, tx, es); err != nil {
			return err
		}
		names, err = recipientNames(ctx, tx, agentIDs(stored))
		return err
	})
	if err != nil {
		return nil, err
	}
	rs := s.receipts(names)
	s.publish(stored)
	return rs, nil
}

// agentIDs returns the distinct agent ids of es.
func agentIDs(es []Entry) []int64 {
	seen := make(map[int64]struct{}, len(es))
	ids := make([]int64, 0, len(es))
	for _, e := range es {
		if _, ok := seen[e.AgentID]; !ok {
			seen[e.AgentID] = struct{}{}
			ids = append(ids, e.AgentID)
		}
	}
	return ids
}
