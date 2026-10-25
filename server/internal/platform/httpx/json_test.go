package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
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

func TestDecodeJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"ok", `{"name":"x"}`, false},
		{"unknown field", `{"name":"x","extra":1}`, true},
		{"malformed", `{`, true},
		{"too large", `{"name":"` + strings.Repeat("a", 1<<20) + `"}`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			var v struct {
				Name string `json:"name"`
			}
			err := DecodeJSON(r, &v)
			if !tc.wantErr {
				if err != nil || v.Name != "x" {
					t.Fatalf("err = %v, v = %+v", err, v)
				}
				return
			}
			if k, ok := apperr.KindOf(err); !ok || k != apperr.Invalid {
				t.Fatalf("err = %v, want invalid", err)
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
