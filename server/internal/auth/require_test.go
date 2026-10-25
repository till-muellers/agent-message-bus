package auth

import (
	"context"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
)

func TestRequirePrincipal(t *testing.T) {
	t.Parallel()
	_, err := RequirePrincipal(context.Background())
	apperrtest.WantKind(t, err, "unauthenticated", "bearer token required")
	want := Principal{Kind: Leader, AgentID: 7, AgentName: "lead"}
	got, err := RequirePrincipal(WithPrincipal(context.Background(), want))
	if err != nil || got != want {
		t.Fatalf("RequirePrincipal = %+v, %v", got, err)
	}
}
