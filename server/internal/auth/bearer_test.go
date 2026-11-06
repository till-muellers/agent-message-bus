package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
)

func TestBearerToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, header, want string
	}{
		{name: "bearer", header: "Bearer amb_x", want: "amb_x"},
		{name: "missing", header: ""},
		{name: "wrong scheme", header: "Basic amb_x"},
		{name: "empty bearer", header: "Bearer "},
		{name: "lower-case scheme", header: "bearer amb_x", want: "amb_x"},
		{name: "upper-case scheme", header: "BEARER amb_x", want: "amb_x"},
		{name: "two spaces", header: "Bearer  amb_x", want: "amb_x"},
		{name: "tab", header: "Bearer\tamb_x", want: "amb_x"},
		{name: "two tokens", header: "Bearer amb_x amb_y"},
		{name: "scheme prefix only", header: "Bearerx amb_x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			got, err := BearerToken(req)
			if tc.want == "" {
				apperrtest.WantKind(t, err, apperr.Unauthenticated, "bearer token required")
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("BearerToken = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}
