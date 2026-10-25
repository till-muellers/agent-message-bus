package config

import (
	"errors"
	"path/filepath"
)

const appDir = "agent-message-bus"

// defaultDataDir returns the per-OS data directory for the bus.
func defaultDataDir(goos string, getenv func(string) string, home string) (string, error) {
	switch goos {
	case "windows":
		if v := getenv("LOCALAPPDATA"); v != "" {
			return filepath.Join(v, appDir), nil
		}
		return "", errors.New("LOCALAPPDATA is not set")
	case "darwin":
		if home == "" {
			return "", errors.New("home directory unknown")
		}
		return filepath.Join(home, "Library", "Application Support", appDir), nil
	default:
		if v := getenv("XDG_DATA_HOME"); v != "" {
			return filepath.Join(v, appDir), nil
		}
		if home == "" {
			return "", errors.New("neither XDG_DATA_HOME nor home directory is set")
		}
		return filepath.Join(home, ".local", "share", appDir), nil
	}
}
