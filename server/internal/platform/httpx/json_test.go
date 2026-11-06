package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

func TestWriteError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{"invalid", apperr.New(apperr.Invalid, "bad"), 400, "invalid", "bad"},
		{"unauthenticated", apperr.New(apperr.Unauthenticated, "who"), 401, "unauthenticated", "who"},
		{"forbidden", apperr.New(apperr.Forbidden, "no"), 403, "forbidden", "no"},
		{"not found", apperr.New(apperr.NotFound, "gone"), 404, "not_found", "gone"},
		{"conflict", fmt.Errorf("wrap: %w", apperr.New(apperr.Conflict, "dup")), 409, "conflict", "dup"},
		{"internal", errors.New("secret db detail"), 500, "internal", "internal error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			WriteError(rec, tc.err)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			var body struct {
				Error struct{ Code, Message string } `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decoding body: %v", err)
			}
			if body.Error.Code != tc.wantCode || body.Error.Message != tc.wantMsg {
				t.Fatalf("body = %+v", body)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusCreated, map[string]int{"a": 1})
	if rec.Code != 201 || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("code=%d ct=%q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"a":1}` {
		t.Fatalf("body = %s", got)
	}
}

// A request the client cancelled is not a server fault: no ERROR line, but the response
// is still the 500 envelope.
func TestWriteErrorLogsACancelledRequestBelowError(t *testing.T) {
	var logs strings.Builder
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	rec := httptest.NewRecorder()
	WriteError(rec, fmt.Errorf("listing agent spaces: %w", context.Canceled))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(logs.String(), "ERROR") {
		t.Fatalf("a cancelled request logged an error: %s", logs.String())
	}
	WriteError(rec, errors.New("real fault"))
	if !strings.Contains(logs.String(), "ERROR") {
		t.Fatalf("a real internal error must still log an error: %q", logs.String())
	}
}
