package mcpserver

import (
	"context"
	"fmt"
	"net/http"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

const principalKey = "principal"

// verifier adapts auth.Service.Authenticate to the SDK bearer-token verifier.
func verifier(a *auth.Service) sdkauth.TokenVerifier {
	return func(ctx context.Context, token string, _ *http.Request) (*sdkauth.TokenInfo, error) {
		p, err := a.Authenticate(ctx, token)
		if k, ok := apperr.KindOf(err); ok && k == apperr.Unauthenticated {
			return nil, fmt.Errorf("%w: %v", sdkauth.ErrInvalidToken, err)
		}
		if err != nil {
			return nil, fmt.Errorf("authenticating: %w", err)
		}
		return &sdkauth.TokenInfo{
			UserID: fmt.Sprintf("%s:%d", p.Kind, p.AgentID),
			Extra:  map[string]any{principalKey: p},
		}, nil
	}
}

// principalOf extracts the principal stored by verifier.
func principalOf(ti *sdkauth.TokenInfo) (auth.Principal, bool) {
	if ti == nil {
		return auth.Principal{}, false
	}
	p, ok := ti.Extra[principalKey].(auth.Principal)
	return p, ok
}
