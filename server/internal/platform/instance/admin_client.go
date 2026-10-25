package instance

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// adminPost POSTs body to url with the admin bearer token and fails on a non-200 status.
func (m Manager) adminPost(ctx context.Context, url, token, body string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := m.Client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return fmt.Errorf("POST %s: status %d: %s", url, res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}
