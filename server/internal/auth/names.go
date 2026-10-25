package auth

import (
	"regexp"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

const namePattern = `^[a-z0-9][a-z0-9_-]{0,62}$`

var nameRE = regexp.MustCompile(namePattern)

// ValidName reports an Invalid error naming field when v is not a valid name.
func ValidName(field, v string) error {
	if !nameRE.MatchString(v) {
		return apperr.New(apperr.Invalid, "%s must match %s", field, namePattern)
	}
	return nil
}
