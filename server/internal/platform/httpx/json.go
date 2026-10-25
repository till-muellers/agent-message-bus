// Package httpx holds shared JSON request/response helpers.
package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

var statusByKind = map[apperr.Kind]int{
	apperr.Invalid:         http.StatusBadRequest,
	apperr.Unauthenticated: http.StatusUnauthorized,
	apperr.Forbidden:       http.StatusForbidden,
	apperr.NotFound:        http.StatusNotFound,
	apperr.Conflict:        http.StatusConflict,
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteJSON writes v as a JSON response with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encoding json response", "err", err)
	}
}

// WriteError maps err to an HTTP status and error body; unknown errors become 500.
func WriteError(w http.ResponseWriter, err error) {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		if status, ok := statusByKind[ae.Kind]; ok {
			WriteJSON(w, status, errorBody{errorDetail{Code: string(ae.Kind), Message: ae.Msg}})
			return
		}
	}
	slog.Error("internal error", "err", err)
	WriteJSON(w, http.StatusInternalServerError, errorBody{errorDetail{Code: "internal", Message: "internal error"}})
}
