package web

import (
	"context"
	"net/http"
	"strconv"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

// Inbox reads the signed-in agent's own mailbox scope (inbox.Service): the read-only
// listing, never the marking read, never an ack and never the stream slot.
type Inbox interface {
	Read(ctx context.Context, p auth.Principal, after int64, limit int) ([]inbox.Entry, error)
	Count(ctx context.Context, p auth.Principal) (int, error)
}

// readInbox answers GET /web/api/inbox?after=&limit= with the entries bus_inbox_read
// would return, but never marks an edge noticed and never acks.
func (g *Web) readInbox(r *http.Request, p auth.Principal) (any, error) {
	after, err := queryInt(r, "after")
	if err != nil {
		return nil, err
	}
	limit, err := queryInt(r, "limit")
	if err != nil {
		return nil, err
	}
	return g.inbox.Read(r.Context(), p, after, int(limit))
}

// queryInt parses query parameter name as an integer; absent means 0.
func queryInt(r *http.Request, name string) (int64, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, apperr.New(apperr.Invalid, "%s must be an integer", name)
	}
	return n, nil
}
