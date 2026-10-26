package web

import (
	"io"
	"net/http"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

// maxBody bounds a mutation's JSON body, like POST /v1/tools.
const maxBody = 1 << 20

// listAdmins answers GET /web/api/admins as bus_list_admins (admin sessions only).
func (g *Web) listAdmins(r *http.Request, p auth.Principal) (any, error) {
	return g.call(r, p, "bus_list_admins", map[string]any{})
}

// mintAdmin answers POST /web/api/admins {"name"} as bus_mint_admin. The token is in this
// response only (Cache-Control: no-store) and is never shown again.
func (g *Web) mintAdmin(r *http.Request, p auth.Principal) (any, error) {
	args, err := readArgs(r)
	if err != nil {
		return nil, err
	}
	return g.callRaw(r, p, "bus_mint_admin", args)
}

// revokeAdmin answers POST /web/api/admins/{name}/revoke as bus_revoke_admin.
func (g *Web) revokeAdmin(r *http.Request, p auth.Principal) (any, error) {
	return g.call(r, p, "bus_revoke_admin", map[string]any{"name": r.PathValue("name")})
}

// readArgs returns r's body as the tool arguments, at most maxBody bytes.
func readArgs(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil {
		return nil, apperr.New(apperr.Invalid, "reading request body: %v", err)
	}
	if len(body) > maxBody {
		return nil, apperr.New(apperr.Invalid, "request body exceeds 1 MiB")
	}
	return body, nil
}
