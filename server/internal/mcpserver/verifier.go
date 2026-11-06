package mcpserver

import (
	"context"
	"fmt"
	"net/http"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
)

const principalKey = "principal"

// verifier hands the principal that auth.Service.Middleware stored in the request context to
// the SDK, which passes it on to the tool handlers. The middleware already rejected every
// request without a valid token with the §0 envelope; a request without a principal can only
// come from wiring without it.
func verifier(_ context.Context, _ string, r *http.Request) (*sdkauth.TokenInfo, error) {
	p, ok := auth.FromContext(r.Context())
	if !ok {
		return nil, sdkauth.ErrInvalidToken
	}
	return &sdkauth.TokenInfo{
		UserID: fmt.Sprintf("%s:%d", p.Kind, p.AgentID),
		Extra:  map[string]any{principalKey: p},
	}, nil
}

// principalOf extracts the principal stored by verifier.
func principalOf(ti *sdkauth.TokenInfo) (auth.Principal, bool) {
	if ti == nil {
		return auth.Principal{}, false
	}
	p, ok := ti.Extra[principalKey].(auth.Principal)
	return p, ok
}
