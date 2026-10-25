// Package invites issues single-use invite codes and redeems them for member tokens.
package invites

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// alphabet has 32 characters without the look-alikes 0, 1, l and o.
const alphabet = "23456789abcdefghijkmnpqrstuvwxyz"

// codeLen is the number of significant characters in a code.
const codeLen = 8

// newCode returns a random code formatted xxxx-xxxx.
func newCode() (string, error) {
	b := make([]byte, codeLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating invite code: %w", err)
	}
	for i, v := range b {
		b[i] = alphabet[v&31]
	}
	return string(b[:4]) + "-" + string(b[4:]), nil
}

// normalize lowercases code and drops every character outside [a-z0-9].
func normalize(code string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, strings.ToLower(code))
}
