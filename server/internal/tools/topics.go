package tools

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/messaging"
)

type directArgs struct {
	Space    string `json:"space"`
	To       string `json:"to"`
	Body     string `json:"body"`
	Critical bool   `json:"critical"`
}

type topicArgs struct {
	Space string `json:"space"`
	Topic string `json:"topic"`
}

type sendTopicArgs struct {
	Space    string `json:"space"`
	Topic    string `json:"topic"`
	Body     string `json:"body"`
	Critical bool   `json:"critical"`
}

func topicTools(m *messaging.Service) []Tool {
	return append(messageTools(m), topicOps(m)...)
}

func messageTools(m *messaging.Service) []Tool {
	return []Tool{
		define("bus_send_direct", "Participants only; critical requires a leader. Delivers the message to the recipient's inbox.",
			okResult, []string{"space", "to", "body"},
			map[string]any{"space": spaceProp(), "to": str(nameDesc("Recipient participant")), "body": bodyProp(), "critical": criticalProp()},
			func(ctx context.Context, p auth.Principal, a directArgs) (any, error) {
				return done(m.SendDirect(ctx, p, a.Space, a.To, a.Body, a.Critical))
			}),
		define("bus_send_topic", "Participants only; only leaders may send to all or send critical. Delivers to every subscriber except the sender.",
			okResult, []string{"space", "topic", "body"},
			map[string]any{"space": spaceProp(), "topic": topicProp(), "body": bodyProp(), "critical": criticalProp()},
			func(ctx context.Context, p auth.Principal, a sendTopicArgs) (any, error) {
				return done(m.SendTopic(ctx, p, a.Space, a.Topic, a.Body, a.Critical))
			}),
		define("bus_list_topics", "Participants only. Lists open topics with the caller's subscription state; no side effects.",
			"[{name, createdBy, subscribed}]",
			spaceReq(), spaceOnly(), func(ctx context.Context, p auth.Principal, a spaceArgs) (any, error) {
				return list(m.ListTopics(ctx, p, a.Space))
			}),
	}
}

func topicOps(m *messaging.Service) []Tool {
	return []Tool{
		topicOp("bus_create_topic", "Participants only. Opens a new topic and subscribes the caller.", m, (*messaging.Service).CreateTopic),
		topicOp("bus_subscribe_topic", "Participants only. Subscribes the caller to the topic; idempotent.", m, (*messaging.Service).Subscribe),
		topicOp("bus_unsubscribe_topic", "Participants only. Stops topic deliveries to the caller; idempotent.", m, (*messaging.Service).Unsubscribe),
		topicOp("bus_close_topic", "Topic creator or leaders only. Closes the topic, notifies subscribers and drops all subscriptions.", m, (*messaging.Service).CloseTopic),
	}
}

// topicOp binds fn to m only when the tool is called.
func topicOp(name, desc string, m *messaging.Service,
	fn func(*messaging.Service, context.Context, auth.Principal, string, string) error) Tool {
	return define(name, desc, okResult, topicReq(), topicProps(), func(ctx context.Context, p auth.Principal, a topicArgs) (any, error) {
		return done(fn(m, ctx, p, a.Space, a.Topic))
	})
}

func topicProps() map[string]any {
	return map[string]any{"space": spaceProp(), "topic": topicProp()}
}
func topicReq() []string { return []string{"space", "topic"} }
