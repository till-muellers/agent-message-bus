package invites

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// OnRevoke cancels the pending invite codes of a revoked name: it deletes every invite for
// name in spaceID inside the revoking transaction. It implements spaces.RevocationHook.
func (s *Service) OnRevoke(ctx context.Context, tx db.DBTX, spaceID int64, name string) (bool, error) {
	return deleteForName(ctx, tx, spaceID, name)
}
