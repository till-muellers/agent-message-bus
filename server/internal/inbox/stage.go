package inbox

import (
	"context"
	"database/sql"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// Staged is what one transaction stored in the inbox; Publish it once that transaction
// has committed.
type Staged struct {
	entries    []Entry
	recipients []recipient
}

// Stage stores es through tx, the caller's transaction, and returns what Publish needs.
func (s *Service) Stage(ctx context.Context, tx db.DBTX, es []Entry) (Staged, error) {
	stored, err := s.insert(ctx, tx, es)
	if err != nil {
		return Staged{}, err
	}
	names, err := recipientNames(ctx, tx, agentIDs(stored))
	if err != nil {
		return Staged{}, err
	}
	return Staged{entries: stored, recipients: names}, nil
}

// Publish wakes the recipients of st and returns one receipt per distinct recipient,
// sorted by name: live when the recipient is connected now, else queued. No entries → [].
// Call it only after the staging transaction committed.
func (s *Service) Publish(st Staged) []Receipt {
	rs := s.receipts(st.recipients)
	s.publish(st.entries)
	return rs
}

// Stager stages entries in a caller's transaction and publishes them after it commits.
type Stager interface {
	Stage(ctx context.Context, tx db.DBTX, es []Entry) (Staged, error)
	Publish(st Staged) []Receipt
}

// InTx runs fn in one transaction on conn, stages the entries fn returns in it and
// publishes them once it commits, returning their receipts. fn MUST use only tx.
func InTx(ctx context.Context, conn *sql.DB, st Stager, fn func(tx db.DBTX) ([]Entry, error)) ([]Receipt, error) {
	var staged Staged
	err := db.InTx(ctx, conn, func(tx *sql.Tx) error {
		es, err := fn(tx)
		if err != nil {
			return err
		}
		staged, err = st.Stage(ctx, tx, es)
		return err
	})
	if err != nil {
		return nil, err
	}
	return st.Publish(staged), nil
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
