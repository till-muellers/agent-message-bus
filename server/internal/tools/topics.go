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
		define("bus_send_direct", "Participants only; critical requires a leader.",
			"Delivers the message to one recipient's inbox. Pitfalls: the recipient must be a participant of the same space, "+
				"and an agent cannot send a direct message to itself.",
			okResult, []string{"space", "to", "body"},
			map[string]any{"space": spaceProp(), "to": str(nameDesc("Recipient participant")), "body": bodyProp(), "critical": criticalProp()},
			func(ctx context.Context, p auth.Principal, a directArgs) (any, error) {
				return done(m.SendDirect(ctx, p, a.Space, a.To, a.Body, a.Critical))
			}),
		define("bus_send_topic", "Participants only; only leaders may send to all or send critical.",
			"Delivers the message to every subscriber of the topic except the sender; every participant receives the all topic.",
			okResult, []string{"space", "topic", "body"},
			map[string]any{"space": spaceProp(), "topic": topicProp(), "body": bodyProp(), "critical": criticalProp()},
			func(ctx context.Context, p auth.Principal, a sendTopicArgs) (any, error) {
				return done(m.SendTopic(ctx, p, a.Space, a.Topic, a.Body, a.Critical))
			}),
		define("bus_list_topics", accessParticipantsRead, "Lists open topics with the caller's subscription state.",
			"[{name, createdBy, subscribed}]",
			spaceReq(), spaceOnly(), func(ctx context.Context, p auth.Principal, a spaceArgs) (any, error) {
				return list(m.ListTopics(ctx, p, a.Space))
			}),
	}
}

func topicOps(m *messaging.Service) []Tool {
	return []Tool{
		topicOp("bus_create_topic", accessParticipants,
			"Opens a new topic and subscribes the caller. Pitfall: nobody else is notified, so subscribers must be told the topic name.",
			m, (*messaging.Service).CreateTopic),
		topicOp("bus_subscribe_topic", accessParticipants,
			"Subscribes the caller to the topic; idempotent. "+
				"Pitfall: the all topic cannot be subscribed or unsubscribed, because everyone receives it.",
			m, (*messaging.Service).Subscribe),
		topicOp("bus_unsubscribe_topic", accessParticipants,
			"Stops topic deliveries to the caller; idempotent. The all topic cannot be unsubscribed.",
			m, (*messaging.Service).Unsubscribe),
		topicOp("bus_close_topic", "The topic's creator or a leader of the space.",
			"Closes the topic, notifies subscribers and drops all subscriptions. "+
				"Pitfall: the all topic cannot be closed, and a closed topic's name can be reused.",
			m, (*messaging.Service).CloseTopic),
	}
}

// topicOp binds fn to m only when the tool is called.
func topicOp(name, access, desc string, m *messaging.Service,
	fn func(*messaging.Service, context.Context, auth.Principal, string, string) error) Tool {
	return define(name, access, desc, okResult, topicReq(), topicProps(), func(ctx context.Context, p auth.Principal, a topicArgs) (any, error) {
		return done(fn(m, ctx, p, a.Space, a.Topic))
	})
}

func topicProps() map[string]any {
	return map[string]any{"space": spaceProp(), "topic": topicProp()}
}
func topicReq() []string { return []string{"space", "topic"} }
