package inbox

import (
	"context"
	"sync/atomic"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// fakeCredentials accepts every principal until revoke, like a token that is deleted or
// rotated while its stream or wait runs.
type fakeCredentials struct{ revoked atomic.Bool }

func (f *fakeCredentials) StillValid(context.Context, db.DBTX, auth.Principal) error {
	if f.revoked.Load() {
		return apperr.New(apperr.Unauthenticated, "invalid token")
	}
	return nil
}

func (f *fakeCredentials) revoke() { f.revoked.Store(true) }
