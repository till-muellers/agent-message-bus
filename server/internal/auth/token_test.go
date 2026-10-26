package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestNewToken(t *testing.T) {
	t.Parallel()
	a, err := NewToken(TokenPrefix)
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	b, err := NewToken(TokenPrefix)
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	if !strings.HasPrefix(a, "amb_") || len(a) != len("amb_")+43 {
		t.Errorf("token %q: want amb_ prefix and 43 base64 chars", a)
	}
	if a == b {
		t.Error("two tokens are equal")
	}
}

func TestHash(t *testing.T) {
	t.Parallel()
	sum := sha256.Sum256([]byte("abc"))
	if got, want := Hash("abc"), hex.EncodeToString(sum[:]); got != want {
		t.Errorf("Hash = %q, want %q", got, want)
	}
}
