package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/tools"
)

// Tools looks up bus tools (tools.Registry). The GUI calls them in-process with the
// session's principal, so rules, decoding and error kinds match /v1/tools.
type Tools interface {
	Get(name string) (tools.Tool, bool)
}

// threads answers GET /web/api/spaces/{space}/blackboards/{board}/threads as bus_list_threads.
func (g *Web) threads(r *http.Request, p auth.Principal) (any, error) {
	return g.call(r, p, "bus_list_threads", map[string]any{"space": r.PathValue("space"), "blackboard": r.PathValue("board")})
}

// thread answers GET /web/api/spaces/{space}/threads/{id} as bus_read_thread; a thread of
// another space is not found.
func (g *Web) thread(r *http.Request, p auth.Principal) (any, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, apperr.New(apperr.Invalid, "threadId must be an integer")
	}
	return g.call(r, p, "bus_read_thread", map[string]any{"space": r.PathValue("space"), "threadId": id})
}

// call runs tool name with args as p.
func (g *Web) call(r *http.Request, p auth.Principal, name string, args map[string]any) (any, error) {
	t, ok := g.tools.Get(name)
	if !ok {
		return nil, fmt.Errorf("tool %s is not registered", name)
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("encoding %s arguments: %w", name, err)
	}
	return t.Call(r.Context(), p, raw)
}
