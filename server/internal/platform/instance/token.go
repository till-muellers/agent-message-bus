package instance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/config"
)

// AdminToken prints the current admin token, generating the token file if missing.
func (m Manager) AdminToken() error {
	tok, err := m.Config.ResolveAdminToken()
	if err != nil {
		return fmt.Errorf("resolving admin token: %w", err)
	}
	_, _ = fmt.Fprintln(m.Out, tok)
	return nil
}

// RotateToken generates a new admin token, swaps it into a live instance, writes the file and prints it.
func (m Manager) RotateToken(ctx context.Context) error {
	if m.Config.AdminToken != "" {
		return errors.New("AMB_ADMIN_TOKEN is set in the environment; rotate it there")
	}
	old, err := m.Config.ResolveAdminToken()
	if err != nil {
		return fmt.Errorf("resolving admin token: %w", err)
	}
	next, err := config.NewAdminToken()
	if err != nil {
		return err
	}
	if err := m.swapLive(ctx, old, next); err != nil {
		return err
	}
	if err := config.WriteTokenFile(m.Config.TokenPath(), next); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(m.Out, next)
	return nil
}

func (m Manager) swapLive(ctx context.Context, old, next string) error {
	st, live, err := Find(ctx, m.Client, m.dir())
	if err != nil || !live {
		return err
	}
	body, err := json.Marshal(map[string]string{"token": next})
	if err != nil {
		return fmt.Errorf("encoding body: %w", err)
	}
	if err := m.adminPost(ctx, st.URL+"/v1/admin/admin-token", old, string(body)); err != nil {
		return fmt.Errorf("rotating token on running instance: %w", err)
	}
	return nil
}
