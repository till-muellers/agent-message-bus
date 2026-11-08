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
		define(BusListBlackboards, accessParticipantsRead, "Lists the space's blackboards with owner and thread count.",
			"[{name, owner, threads}]",
			spaceReq(), spaceOnly(), func(ctx context.Context, p auth.Principal, a spaceArgs) (any, error) {
				return list(b.List(ctx, p, a.Space))
			}),
		boardOp(BusCreateBlackboard, accessParticipants,
			"Creates a blackboard owned by the caller. Pitfall: a member's boards are deleted when it leaves or is revoked.",
			newBoardProps(), b, (*blackboards.Service).Create),
		boardOp(BusDeleteBlackboard, "The board's owner, leaders of the space and admins.",
			"Deletes the blackboard with its threads and notifies thread subscribers.", boardProps(), b, (*blackboards.Service).Delete),
		define(BusListThreads, accessParticipantsRead, "Lists the blackboard's threads, most recently active first.",
			"[{id, title, createdBy, posts, lastPostAt}]",
			boardReq(), boardProps(), func(ctx context.Context, p auth.Principal, a boardArgs) (any, error) {
				return list(b.ListThreads(ctx, p, a.Space, a.Blackboard))
			}),
		define(BusCreateThread, accessParticipants,
			"Creates a thread with its first post. Pitfall: the creator is subscribed and gets later posts by others, "+
				"unless it is an admin that has not joined the space.",
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
func boardOp(name, access, desc string, props map[string]any, b *blackboards.Service,
	fn func(*blackboards.Service, context.Context, auth.Principal, string, string) error) Tool {
	return define(name, access, desc, okResult, boardReq(), props, func(ctx context.Context, p auth.Principal, a boardArgs) (any, error) {
		return done(fn(b, ctx, p, a.Space, a.Blackboard))
	})
}

func threadTools(b *blackboards.Service) []Tool {
	return []Tool{
		define(BusReadThread, accessParticipantsRead, "Returns the thread with all posts.",
			"{id, blackboard, title, createdBy, subscribed, posts: [{id, author, body, createdAt}]}",
			threadReq(), threadProps(), func(ctx context.Context, p auth.Principal, a threadArgs) (any, error) {
				return b.ReadThread(ctx, p, a.Space, a.ThreadID)
			}),
		threadOp(BusRemoveThread, "The thread's creator, leaders of the space and admins.",
			"Deletes the thread and notifies its subscribers.", b, (*blackboards.Service).RemoveThread),
		threadOp(BusSubscribeThread, accessSubscribers,
			"Subscribes the caller to new posts in the thread; idempotent.", b, (*blackboards.Service).Subscribe),
		threadOp(BusUnsubscribeThread, accessSubscribers,
			"Stops post notifications for the thread; idempotent.", b, (*blackboards.Service).Unsubscribe),
		define(BusAddPost, accessParticipants,
			"Appends a post and notifies the thread's subscribers except the author.",
			"{postId, receipts: [{to, delivery: live|queued}]}", []string{"space", "threadId", "body"},
			map[string]any{"space": spaceProp(), "threadId": threadProp(), "body": bodyProp()},
			func(ctx context.Context, p auth.Principal, a postArgs) (any, error) {
				id, rs, err := b.AddPost(ctx, p, a.Space, a.ThreadID, a.Body)
				if err != nil {
					return nil, err
				}
				return map[string]any{"postId": id, "receipts": rs}, nil
			}),
	}
}

// threadOp binds fn to b only when the tool is called.
func threadOp(name, access, desc string, b *blackboards.Service,
	fn func(*blackboards.Service, context.Context, auth.Principal, string, int64) error) Tool {
	return define(name, access, desc, okResult, threadReq(), threadProps(), func(ctx context.Context, p auth.Principal, a threadArgs) (any, error) {
		return done(fn(b, ctx, p, a.Space, a.ThreadID))
	})
}

func boardProps() map[string]any {
	return map[string]any{"space": spaceProp(), "blackboard": boardProp()}
}
func newBoardProps() map[string]any {
	return map[string]any{"space": spaceProp(), "blackboard": newNameProp("Blackboard")}
}
func boardReq() []string { return []string{"space", "blackboard"} }
func threadProps() map[string]any {
	return map[string]any{"space": spaceProp(), "threadId": threadProp()}
}
func threadReq() []string { return []string{"space", "threadId"} }
