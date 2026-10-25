// Package config loads server settings from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const (
	minTokenLen = 32
	dbFileName  = "agent-message-bus.db"
)

// Config holds the server settings.
type Config struct{ Addr, DBPath, AdminToken string }

// DataDir returns the directory holding the database; log and pid files live there too.
func (c Config) DataDir() string { return filepath.Dir(c.DBPath) }

// Load reads settings via getenv, applying per-OS defaults and validating the admin token.
func Load(getenv func(string) string) (Config, error) {
	return load(getenv, runtime.GOOS, os.UserHomeDir)
}

func load(getenv func(string) string, goos string, home func() (string, error)) (Config, error) {
	c := Config{Addr: orDefault(getenv("AMB_ADDR"), ":8080"), DBPath: getenv("AMB_DB_PATH"), AdminToken: getenv("AMB_ADMIN_TOKEN")}
	if c.AdminToken == "" {
		return Config{}, errors.New("AMB_ADMIN_TOKEN is required")
	}
	if len(c.AdminToken) < minTokenLen {
		return Config{}, errors.New("AMB_ADMIN_TOKEN must be at least 32 characters")
	}
	if c.DBPath == "" {
		h, _ := home() // only needed by some fallbacks; defaultDataDir reports when it is missing
		dir, err := defaultDataDir(goos, getenv, h)
		if err != nil {
			return Config{}, fmt.Errorf("default AMB_DB_PATH: %w", err)
		}
		c.DBPath = filepath.Join(dir, dbFileName)
	}
	return c, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
