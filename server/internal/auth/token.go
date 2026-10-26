package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// Token prefixes: a label for humans and secret scanners only; authority comes from the agent row.
const (
	TokenPrefix      = "amb_"
	AdminTokenPrefix = "amb_admin_"
)

// NewToken returns prefix followed by 32 random bytes, RawURL base64 encoded.
func NewToken(prefix string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// Hash returns the hex SHA-256 of token.
func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
