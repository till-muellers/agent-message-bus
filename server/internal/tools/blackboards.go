package tools

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/blackboards"
)

type boardArgs struct {
	Space      string `json:"space"`
	Blackboard string `json:"blackboard"`
}

type createThreadArgs struct {
	Space      string `json:"space"`
	Blackboard string `json:"blackboard"`
	Title      string `json:"title"`
	Body       string `json:"body"`
}

type threadArgs struct {
	Space    string `json:"space"`
	ThreadID int64  `json:"threadId"`
}

type postArgs struct {
	Space    string `json:"space"`
	ThreadID int64  `json:"threadId"`
	Body     string `json:"body"`
}

func blackboardTools(b *blackboards.Service) []Tool {
	return append(boardTools(b), threadTools(b)...)
}

func boardTools(b *blackboards.Service) []Tool {
	return []Tool{
		define("bus_list_blackboards", "Participants only. Lists the space's blackboards with owner and thread count; no side effects.",
			"[{name, owner, threads}]",
			spaceReq(), spaceOnly(), func(ctx context.Context, p auth.Principal, a spaceArgs) (any, error) {
				return list(b.List(ctx, p, a.Space))
			}),
		boardOp("bus_create_blackboard", "Participants only. Creates a blackboard owned by the caller.", b, (*blackboards.Service).Create),
		boardOp("bus_delete_blackboard", "Owner or leaders only. Deletes the blackboard with its threads and notifies thread subscribers.", b, (*blackboards.Service).Delete),
		define("bus_list_threads", "Participants only. Lists the blackboard's threads, most recently active first; no side effects.",
			"[{id, title, createdBy, posts, lastPostAt}]",
			boardReq(), boardProps(), func(ctx context.Context, p auth.Principal, a boardArgs) (any, error) {
				return list(b.ListThreads(ctx, p, a.Space, a.Blackboard))
			}),
		define("bus_create_thread", "Participants only. Creates a thread with its first post and subscribes the caller.",
			"{threadId}", []string{"space", "blackboard", "title", "body"},
			map[string]any{"space": spaceProp(), "blackboard": boardProp(), "title": titleProp(), "body": bodyProp()},
			func(ctx context.Context, p auth.Principal, a createThreadArgs) (any, error) {
				id, err := b.CreateThread(ctx, p, a.Space, a.Blackboard, a.Title, a.Body)
				if err != nil {
					return nil, err
				}
				return map[string]int64{"threadId": id}, nil
			}),
	}
}

// boardOp binds fn to b only when the tool is called.
func boardOp(name, desc string, b *blackboards.Service,
	fn func(*blackboards.Service, context.Context, auth.Principal, string, string) error) Tool {
	return define(name, desc, okResult, boardReq(), boardProps(), func(ctx context.Context, p auth.Principal, a boardArgs) (any, error) {
		return done(fn(b, ctx, p, a.Space, a.Blackboard))
	})
}

func threadTools(b *blackboards.Service) []Tool {
	return []Tool{
		define("bus_read_thread", "Participants only. Returns the thread with all posts; no side effects.",
			"{id, blackboard, title, createdBy, subscribed, posts: [{id, author, body, createdAt}]}",
			threadReq(), threadProps(), func(ctx context.Context, p auth.Principal, a threadArgs) (any, error) {
				return b.ReadThread(ctx, p, a.Space, a.ThreadID)
			}),
		threadOp("bus_remove_thread", "Thread creator or leaders only. Deletes the thread and notifies its subscribers.", b, (*blackboards.Service).RemoveThread),
		threadOp("bus_subscribe_thread", "Participants only. Subscribes the caller to new posts in the thread; idempotent.", b, (*blackboards.Service).Subscribe),
		threadOp("bus_unsubscribe_thread", "Participants only. Stops post notifications for the thread; idempotent.", b, (*blackboards.Service).Unsubscribe),
		define("bus_add_post", "Participants only. Appends a post and notifies the thread's subscribers except the author.",
			"{postId}", []string{"space", "threadId", "body"},
			map[string]any{"space": spaceProp(), "threadId": threadProp(), "body": bodyProp()},
			func(ctx context.Context, p auth.Principal, a postArgs) (any, error) {
				id, err := b.AddPost(ctx, p, a.Space, a.ThreadID, a.Body)
				if err != nil {
					return nil, err
				}
				return map[string]int64{"postId": id}, nil
			}),
	}
}

// threadOp binds fn to b only when the tool is called.
func threadOp(name, desc string, b *blackboards.Service,
	fn func(*blackboards.Service, context.Context, auth.Principal, string, int64) error) Tool {
	return define(name, desc, okResult, threadReq(), threadProps(), func(ctx context.Context, p auth.Principal, a threadArgs) (any, error) {
		return done(fn(b, ctx, p, a.Space, a.ThreadID))
	})
}

func boardProps() map[string]any {
	return map[string]any{"space": spaceProp(), "blackboard": boardProp()}
}
func boardReq() []string { return []string{"space", "blackboard"} }
func threadProps() map[string]any {
	return map[string]any{"space": spaceProp(), "threadId": threadProp()}
}
func threadReq() []string { return []string{"space", "threadId"} }
