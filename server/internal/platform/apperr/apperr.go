// Package apperr defines the typed errors every feature returns to callers.
package apperr

import (
	"errors"
	"fmt"
)

// Kind classifies an error for transport mapping (HTTP status, MCP error text).
type Kind string

const (
	Invalid         Kind = "invalid"
	Unauthenticated Kind = "unauthenticated"
	Forbidden       Kind = "forbidden"
	NotFound        Kind = "not_found"
	Conflict        Kind = "conflict"
)

// Error is a caller-facing error with a stable kind and a human message.
type Error struct {
	Kind Kind
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

// New builds an *Error with a formatted message.
func New(k Kind, format string, args ...any) *Error {
	return &Error{Kind: k, Msg: fmt.Sprintf(format, args...)}
}

// KindOf reports the kind of the first *Error in err's chain.
func KindOf(err error) (Kind, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind, true
	}
	return "", false
}
