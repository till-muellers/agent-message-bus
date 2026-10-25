// Package config loads server settings from the environment.
package config

import "errors"

const minTokenLen = 32

// Config holds the server settings.
type Config struct{ Addr, DBPath, AdminToken string }

// Load reads settings via getenv, applying defaults and validating the admin token.
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		Addr:       orDefault(getenv("AMB_ADDR"), ":8080"),
		DBPath:     orDefault(getenv("AMB_DB_PATH"), "/data/agent-message-bus.db"),
		AdminToken: getenv("AMB_ADMIN_TOKEN"),
	}
	if c.AdminToken == "" {
		return Config{}, errors.New("AMB_ADMIN_TOKEN is required")
	}
	if len(c.AdminToken) < minTokenLen {
		return Config{}, errors.New("AMB_ADMIN_TOKEN must be at least 32 characters")
	}
	return c, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
