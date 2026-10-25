package auth

import (
	"context"
	"database/sql"
)

// Kind is the class of an authenticated caller.
type Kind string

const (
	Admin  Kind = "admin"
	Leader Kind = "leader"
	Member Kind = "member"
)

// Principal is the authenticated caller. SpaceID is set only for members.
type Principal struct {
	Kind      Kind
	AgentID   int64
	AgentName string
	SpaceID   int64
}

// DBTX is satisfied by both *sql.DB and *sql.Tx.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
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
