package domain

import (
	"strings"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

const (
	// MinAdminTokenLen is the minimum length of an instance admin token.
	MinAdminTokenLen = 32
	// MaxAdminTokenLen keeps an admin token far below the request header limit.
	MaxAdminTokenLen = 512
)

// AdminToken returns tok without surrounding whitespace, or Invalid unless that is
// MinAdminTokenLen..MaxAdminTokenLen visible ASCII characters (0x21-0x7E). The rule makes
// every accepted token presentable in "Authorization: Bearer <token>": HTTP drops
// surrounding whitespace and rejects control characters in header values. It guards every
// way to set the instance admin token: AMB_ADMIN_TOKEN, the token file and the token swap.
func AdminToken(tok string) (string, error) {
	tok = strings.TrimSpace(tok)
	if len(tok) < MinAdminTokenLen || len(tok) > MaxAdminTokenLen || !visibleASCII(tok) {
		return "", apperr.New(apperr.Invalid, "admin token must be %d..%d visible ASCII characters",
			MinAdminTokenLen, MaxAdminTokenLen)
	}
	return tok, nil
}

func visibleASCII(s string) bool {
	for i := range len(s) {
		if s[i] < 0x21 || s[i] > 0x7e {
			return false
		}
	}
	return true
}
