package admin

import (
	"errors"
	"io"
	"net/http"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpx"
	"github.com/till-muellers/agent-message-bus/server/internal/tools"
)

// maxTokenBody bounds the token swap body; an admin token is at most 512 characters.
const maxTokenBody = 4 << 10

// TokenSetter swaps the admin credential.
type TokenSetter interface {
	SetAdminToken(token string) error
}

// Waker wakes the streams and waits of agents so they re-validate their token.
type Waker interface {
	Wake(agentIDs ...int64)
}

type okBody struct {
	OK bool `json:"ok"`
}

type tokenBody struct {
	Token string `json:"token"`
}

// Mount registers POST /v1/admin/shutdown (any admin) and POST /v1/admin/admin-token (the
// bootstrap admin only) on mux; a token swap wakes the bootstrap admin's streams and waits
// through wk, so those on the replaced token end at once. authn must authenticate the
// bearer token and store the principal (auth.Service.Middleware).
func Mount(mux *http.ServeMux, authn func(http.Handler) http.Handler, ts TokenSetter, wk Waker, shutdown func()) {
	mux.Handle("POST /v1/admin/shutdown", authn(guard(shutdownHandler(shutdown))))
	mux.Handle("POST /v1/admin/admin-token", authn(guard(bootstrapOnly(tokenHandler(ts, wk)))))
}

func shutdownHandler(shutdown func()) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, okBody{OK: true})
		shutdown()
	}
}

// tokenHandler decodes the body like every other JSON body (tools.Decode), swaps the token
// and wakes the caller's agent, the bootstrap admin, whose old credential just died.
func tokenHandler(ts TokenSetter, wk Waker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		in, err := readTokenBody(w, r)
		if err == nil {
			err = ts.SetAdminToken(in.Token)
		}
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		if p, ok := auth.FromContext(r.Context()); ok {
			wk.Wake(p.AgentID)
		}
		httpx.WriteJSON(w, http.StatusOK, okBody{OK: true})
	}
}

func readTokenBody(w http.ResponseWriter, r *http.Request) (tokenBody, error) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxTokenBody))
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		return tokenBody{}, apperr.New(apperr.Invalid, "request body exceeds 4 KiB")
	case err != nil:
		return tokenBody{}, apperr.New(apperr.Invalid, "reading request body: %v", err)
	}
	return tools.Decode[tokenBody](raw)
}
