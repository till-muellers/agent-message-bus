package auth

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

// Kind is the class of an authenticated caller.
type Kind string

const (
	Admin  Kind = "admin"
	Leader Kind = "leader"
	Member Kind = "member"
)

// Principal is the authenticated caller. SpaceID is set only for members; Bootstrap
// is true only for the instance admin token.
type Principal struct {
	Kind      Kind
	AgentID   int64
	AgentName string
	SpaceID   int64
	Bootstrap bool
}

type principalKey struct{}

// WithPrincipal stores p in ctx.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// FromContext returns the principal stored by WithPrincipal.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// RequirePrincipal returns the principal in ctx or Unauthenticated "bearer token required".
func RequirePrincipal(ctx context.Context) (Principal, error) {
	p, ok := FromContext(ctx)
	if !ok {
		return Principal{}, apperr.New(apperr.Unauthenticated, "bearer token required")
	}
	return p, nil
}
