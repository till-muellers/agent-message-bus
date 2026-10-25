package invites

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// Revocation implements spaces.RevocationHook: revoking a name cancels its pending invite codes.
// It holds no dependencies, so spaces can take it before the invites Service exists.
type Revocation struct{}

// OnRevoke deletes every invite for name in spaceID inside the revoking transaction.
func (Revocation) OnRevoke(ctx context.Context, tx db.DBTX, spaceID int64, name string) (bool, error) {
	return deleteForName(ctx, tx, spaceID, name)
}
