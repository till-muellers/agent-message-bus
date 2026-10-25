package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const tokenFileName = "admin-token"

// TokenPath returns the admin token file inside the data dir.
func (c Config) TokenPath() string { return filepath.Join(c.DataDir(), tokenFileName) }

// ResolveAdminToken returns the AMB_ADMIN_TOKEN override, else the token file, generating it if missing.
func (c Config) ResolveAdminToken() (string, error) {
	if c.AdminToken != "" {
		return c.AdminToken, nil
	}
	b, err := os.ReadFile(c.TokenPath())
	if err == nil {
		return validFileToken(c.TokenPath(), strings.TrimSpace(string(b)))
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("reading admin token %s: %w", c.TokenPath(), err)
	}
	tok, err := NewAdminToken()
	if err != nil {
		return "", err
	}
	if err := WriteTokenFile(c.TokenPath(), tok); err != nil {
		return "", err
	}
	return tok, nil
}

func validFileToken(path, tok string) (string, error) {
	if len(tok) < minTokenLen {
		return "", fmt.Errorf("admin token in %s must be at least 32 characters", path)
	}
	return tok, nil
}

// NewAdminToken returns "amb_admin_" followed by 32 random bytes, RawURL base64 encoded.
func NewAdminToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}
	return "amb_admin_" + base64.RawURLEncoding.EncodeToString(b), nil
}

// WriteTokenFile writes tok plus a newline to path with mode 0600, creating the directory.
func WriteTokenFile(path, tok string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("creating dir for %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(tok+"\n"), 0o600); err != nil {
		return fmt.Errorf("writing admin token %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil { // tightens a pre-existing file; best effort on Windows
		return fmt.Errorf("chmod admin token %s: %w", path, err)
	}
	return nil
}
