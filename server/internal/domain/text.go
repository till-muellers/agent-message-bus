// Package domain holds business rules shared by several features.
package domain

import (
	"regexp"
	"unicode/utf8"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

const (
	// NamePattern is the rule for agent, space, topic and blackboard names.
	NamePattern = `^[a-z0-9][a-z0-9_-]{0,62}$`
	// MaxBodyBytes bounds message and post bodies.
	MaxBodyBytes = 65536
	// MaxTitleChars bounds thread titles, counted in characters.
	MaxTitleChars = 200
)

var nameRE = regexp.MustCompile(NamePattern)

// ValidName reports an Invalid error naming field when v breaks NamePattern.
func ValidName(field, v string) error {
	if !nameRE.MatchString(v) {
		return apperr.New(apperr.Invalid, "%s must match %s", field, NamePattern)
	}
	return nil
}

// ValidBody requires 1..MaxBodyBytes bytes.
func ValidBody(body string) error {
	return ValidText("body", body)
}

// ValidText requires 1..MaxBodyBytes bytes and names field in the Invalid error.
func ValidText(field, v string) error {
	if len(v) < 1 || len(v) > MaxBodyBytes {
		return apperr.New(apperr.Invalid, "%s must be 1..%d bytes", field, MaxBodyBytes)
	}
	return nil
}

// ValidTitle requires 1..MaxTitleChars characters.
func ValidTitle(title string) error {
	if n := utf8.RuneCountInString(title); n < 1 || n > MaxTitleChars {
		return apperr.New(apperr.Invalid, "title must be 1..%d characters", MaxTitleChars)
	}
	return nil
}
