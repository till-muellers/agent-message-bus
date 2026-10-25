package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

func TestSetAdminToken(t *testing.T) {
	t.Parallel()
	next := strings.Repeat("n", 32)
	tests := []struct {
		name, token       string
		wantErr           bool
		oldWorks, newWork bool
	}{
		{name: "swap", token: next, oldWorks: false, newWork: true},
		{name: "too short", token: "short", wantErr: true, oldWorks: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, _ := newTestService(t)
			err := s.SetAdminToken(tc.token)
			if tc.wantErr {
				if k, _ := apperr.KindOf(err); k != apperr.Invalid {
					t.Fatalf("err = %v, want invalid", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if got := isAdmin(s, testAdmin); got != tc.oldWorks {
				t.Fatalf("old token admin = %v, want %v", got, tc.oldWorks)
			}
			if got := isAdmin(s, tc.token); got != tc.newWork {
				t.Fatalf("new token admin = %v, want %v", got, tc.newWork)
			}
		})
	}
}

func isAdmin(s *Service, tok string) bool {
	p, err := s.Authenticate(context.Background(), tok)
	return err == nil && p.Kind == Admin
}
