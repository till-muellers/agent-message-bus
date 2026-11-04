package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

// inviteBody is POST /web/api/spaces/{space}/invites.
type inviteBody struct {
	Name string  `json:"name"`
	Task *string `json:"task"`
}

// revokeBody is POST /web/api/spaces/{space}/revoke; Confirm must be "<space>/<name>".
type revokeBody struct {
	Name    string `json:"name"`
	Confirm string `json:"confirm"`
}

// postBody is POST /web/api/spaces/{space}/threads/{id}/posts.
type postBody struct {
	Body string `json:"body"`
}

// messageBody is POST /web/api/spaces/{space}/topics/{topic}/messages.
type messageBody struct {
	Body     string `json:"body"`
	Critical bool   `json:"critical"`
}

// deleteBody is DELETE /web/api/spaces/{space}; Confirm must be "<space>".
type deleteBody struct {
	Confirm string `json:"confirm"`
}

// invite answers POST /web/api/spaces/{space}/invites as bus_invite_member; the code is in
// this response only.
func (g *Web) invite(r *http.Request, p auth.Principal) (any, error) {
	in, err := decodeBody[inviteBody](r)
	if err != nil {
		return nil, err
	}
	space := r.PathValue("space")
	args := map[string]any{"space": space, "name": in.Name}
	if in.Task != nil {
		args["task"] = *in.Task
	}
	return g.mutate(r, p, "bus_invite_member", args, mutationLog{"invite", space, in.Name})
}

// revoke answers POST /web/api/spaces/{space}/revoke as bus_revoke_member once the body
// confirms the space and the name.
func (g *Web) revoke(r *http.Request, p auth.Principal) (any, error) {
	in, err := decodeBody[revokeBody](r)
	if err != nil {
		return nil, err
	}
	space := r.PathValue("space")
	if in.Confirm != space+"/"+in.Name {
		return nil, apperr.New(apperr.Invalid, "confirm the revoke of %s in space %s", in.Name, space)
	}
	return g.mutate(r, p, "bus_revoke_member", map[string]any{"space": space, "name": in.Name},
		mutationLog{"revoke", space, in.Name})
}

// post answers POST /web/api/spaces/{space}/threads/{id}/posts as bus_add_post.
func (g *Web) post(r *http.Request, p auth.Principal) (any, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, apperr.New(apperr.Invalid, "threadId must be an integer")
	}
	in, err := decodeBody[postBody](r)
	if err != nil {
		return nil, err
	}
	space := r.PathValue("space")
	args := map[string]any{"space": space, "threadId": id, "body": in.Body}
	return g.mutate(r, p, "bus_add_post", args, mutationLog{"post", space, r.PathValue("id")})
}

// send answers POST /web/api/spaces/{space}/topics/{topic}/messages as bus_send_topic.
func (g *Web) send(r *http.Request, p auth.Principal) (any, error) {
	in, err := decodeBody[messageBody](r)
	if err != nil {
		return nil, err
	}
	space, topic := r.PathValue("space"), r.PathValue("topic")
	args := map[string]any{"space": space, "topic": topic, "body": in.Body, "critical": in.Critical}
	return g.mutate(r, p, "bus_send_topic", args, mutationLog{"send", space, topic})
}

// deleteSpace answers DELETE /web/api/spaces/{space} as bus_delete_space once the body
// confirms the space.
func (g *Web) deleteSpace(r *http.Request, p auth.Principal) (any, error) {
	in, err := decodeBody[deleteBody](r)
	if err != nil {
		return nil, err
	}
	space := r.PathValue("space")
	if in.Confirm != space {
		return nil, apperr.New(apperr.Invalid, "confirm the deletion of space %s", space)
	}
	return g.mutate(r, p, "bus_delete_space", map[string]any{"space": space},
		mutationLog{"delete-space", space, space})
}

// mutationLog is the audit line of one GUI mutation (PO-1: a log line, no table).
type mutationLog struct {
	action, space, target string
}

// mutate runs tool name with args as p and, when it succeeds, writes the mutation's line.
func (g *Web) mutate(r *http.Request, p auth.Principal, name string, args map[string]any, l mutationLog) (any, error) {
	out, err := g.call(r, p, name, args)
	if err != nil {
		return nil, err
	}
	g.logMutation(r.Context(), p, l)
	return out, nil
}

// logMutation writes msg="web mutation" with the acting agent; never a token.
func (g *Web) logMutation(ctx context.Context, p auth.Principal, l mutationLog) {
	g.log.InfoContext(ctx, "web mutation", "actor", p.AgentName, "action", l.action, "space", l.space, "target", l.target)
}

// decodeBody strictly decodes r's JSON body into T; an empty body is the zero value.
func decodeBody[T any](r *http.Request) (T, error) {
	var v T
	raw, err := readArgs(r)
	if err != nil {
		return v, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return v, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return v, apperr.New(apperr.Invalid, "invalid request body: %v", err)
	}
	return v, nil
}
