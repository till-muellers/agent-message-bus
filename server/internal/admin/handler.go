package admin

import (
	"encoding/json"
	"net/http"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpx"
)

// TokenSetter swaps the admin credential.
type TokenSetter interface {
	SetAdminToken(token string) error
}

type okBody struct {
	OK bool `json:"ok"`
}

// Mount registers POST /v1/admin/shutdown and POST /v1/admin/admin-token on mux.
// authn must authenticate the bearer token and store the principal (auth.Service.Middleware).
func Mount(mux *http.ServeMux, authn func(http.Handler) http.Handler, ts TokenSetter, shutdown func()) {
	mux.Handle("POST /v1/admin/shutdown", authn(guard(shutdownHandler(shutdown))))
	mux.Handle("POST /v1/admin/admin-token", authn(guard(tokenHandler(ts))))
}

func shutdownHandler(shutdown func()) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, okBody{OK: true})
		shutdown()
	}
}

func tokenHandler(ts TokenSetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpx.WriteError(w, apperr.New(apperr.Invalid, "invalid JSON body: %v", err))
			return
		}
		if err := ts.SetAdminToken(in.Token); err != nil {
			httpx.WriteError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, okBody{OK: true})
	}
}
