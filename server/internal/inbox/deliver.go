package inbox

import (
	"context"
	"database/sql"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// Deliver runs fn in a transaction, inserts the entries it returns in the same
// transaction and publishes them once it commits. fn MUST use only tx.
func (s *Service) Deliver(ctx context.Context, fn func(tx *sql.Tx) ([]Entry, error)) error {
	var stored []Entry
	err := db.InTx(ctx, s.db, func(tx *sql.Tx) error {
		es, err := fn(tx)
		if err != nil {
			return err
		}
		stored, err = s.insert(ctx, tx, es)
		return err
	})
	if err != nil {
		return err
	}
	s.publish(stored)
	return nil
}
