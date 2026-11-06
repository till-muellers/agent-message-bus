package inbox

import (
	"context"
	"sync/atomic"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// fakeCredentials accepts every principal until revoke, like a token that is deleted or
// rotated while its stream or wait runs. checks counts the re-validations; afterFirstCheck,
// when set before the stream or wait starts, runs right after the first check passed, like
// a revocation that commits just after it.
type fakeCredentials struct {
	revoked         atomic.Bool
	checks          atomic.Int64
	afterFirstCheck func()
}

func (f *fakeCredentials) StillValid(context.Context, db.DBTX, auth.Principal) error {
	revoked := f.revoked.Load()
	n := f.checks.Add(1) // counted after the load: a revoke seen as done after check n hits a later one
	if revoked {
		return apperr.New(apperr.Unauthenticated, "invalid token")
	}
	if n == 1 && f.afterFirstCheck != nil {
		f.afterFirstCheck()
	}
	return nil
}

func (f *fakeCredentials) revoke() { f.revoked.Store(true) }
