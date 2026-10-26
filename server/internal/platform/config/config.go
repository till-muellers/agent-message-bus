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
	// DefaultAddr is the listen address used when AMB_ADDR is unset.
	DefaultAddr = "127.0.0.1:8080"
	// DefaultWebAddr is the web GUI listen address used when AMB_WEB_ADDR is unset.
	DefaultWebAddr = "127.0.0.1:8081"
	webOff         = "off"
)

// Config holds the server settings. AdminToken is the AMB_ADMIN_TOKEN override, empty when
// unset. WebAddr is the web GUI listen address, empty when the GUI is disabled.
type Config struct{ Addr, WebAddr, DBPath, AdminToken string }

// DataDir returns the directory holding the database; log, state and token files live there too.
func (c Config) DataDir() string { return filepath.Dir(c.DBPath) }

// Load reads settings via lookup (os.LookupEnv), applying per-OS defaults and validating
// an admin token override and the web GUI address.
func Load(lookup func(string) (string, bool)) (Config, error) {
	return load(lookup, runtime.GOOS, os.UserHomeDir)
}

func load(lookup func(string) (string, bool), goos string, home func() (string, error)) (Config, error) {
	getenv := func(k string) string { v, _ := lookup(k); return v }
	c := Config{Addr: orDefault(getenv("AMB_ADDR"), DefaultAddr), WebAddr: webAddr(lookup),
		DBPath: getenv("AMB_DB_PATH"), AdminToken: getenv("AMB_ADMIN_TOKEN")}
	if c.AdminToken != "" && len(c.AdminToken) < minTokenLen {
		return Config{}, errors.New("AMB_ADMIN_TOKEN must be at least 32 characters")
	}
	if c.WebAddr != "" && c.WebAddr == c.Addr {
		return Config{}, fmt.Errorf("AMB_WEB_ADDR %s must differ from AMB_ADDR; set another address or off", c.WebAddr)
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

// webAddr returns AMB_WEB_ADDR: DefaultWebAddr when unset, "" (disabled) when empty or "off".
func webAddr(lookup func(string) (string, bool)) string {
	v, ok := lookup("AMB_WEB_ADDR")
	switch {
	case !ok:
		return DefaultWebAddr
	case v == webOff:
		return ""
	}
	return v
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
