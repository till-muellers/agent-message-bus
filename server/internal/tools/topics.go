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
		define(BusSendDirect, "Participants and admins; critical requires a leader or an admin.",
			"Sends a message to one participant of the space; use bus_send_topic for several. "+
				"You cannot message yourself, and an admin that has not joined the space is not a participant. Receipts show `live` or `queued` (not connected).",
			receiptsResult, []string{"space", "to", "body"},
			map[string]any{"space": spaceProp(), "to": nameProp("Recipient participant"), "body": bodyProp(), "critical": criticalProp()},
			func(ctx context.Context, p auth.Principal, a directArgs) (any, error) {
				return sent(m.SendDirect(ctx, p, a.Space, a.To, a.Body, a.Critical))
			}),
		define(BusSendTopic, "Participants and admins; only leaders and admins may send to all or send critical.",
			"Sends a message to every subscriber of the topic except you; every participant gets `all`. Receipts show each subscriber as `live` or `queued` (not connected).",
			receiptsResult, []string{"space", "topic", "body"},
			map[string]any{"space": spaceProp(), "topic": topicProp(), "body": bodyProp(), "critical": criticalProp()},
			func(ctx context.Context, p auth.Principal, a sendTopicArgs) (any, error) {
				return sent(m.SendTopic(ctx, p, a.Space, a.Topic, a.Body, a.Critical))
			}),
		define(BusListTopics, accessParticipantsRead, "Lists open topics with the caller's subscription state.",
			"[{name, createdBy, subscribed}]",
			spaceReq(), spaceOnly(), func(ctx context.Context, p auth.Principal, a spaceArgs) (any, error) {
				return list(m.ListTopics(ctx, p, a.Space))
			}),
	}
}

func topicOps(m *messaging.Service) []Tool {
	return []Tool{
		topicOp(BusCreateTopic, accessParticipants,
			"Opens a topic and subscribes you (not an admin that has not joined the space). "+
				"Nobody else is notified: tell them the topic name.",
			newTopicProps(), m, (*messaging.Service).CreateTopic),
		topicOp(BusSubscribeTopic, accessSubscribers,
			"Subscribes you to the topic; idempotent. `all` needs no subscription and cannot be subscribed or unsubscribed.",
			topicProps(), m, (*messaging.Service).Subscribe),
		topicOp(BusUnsubscribeTopic, accessSubscribers,
			"Stops topic deliveries to you; idempotent. `all` cannot be unsubscribed.",
			topicProps(), m, (*messaging.Service).Unsubscribe),
		topicOp(BusCloseTopic, "The topic's creator, leaders of the space and admins.",
			"Closes the topic, notifies subscribers and drops all subscriptions; the name can be reused. `all` cannot be closed.",
			topicProps(), m, (*messaging.Service).CloseTopic),
	}
}

// topicOp binds fn to m only when the tool is called.
func topicOp(name, access, desc string, props map[string]any, m *messaging.Service,
	fn func(*messaging.Service, context.Context, auth.Principal, string, string) error) Tool {
	return define(name, access, desc, okResult, topicReq(), props, func(ctx context.Context, p auth.Principal, a topicArgs) (any, error) {
		return done(fn(m, ctx, p, a.Space, a.Topic))
	})
}

func topicProps() map[string]any {
	return map[string]any{"space": spaceProp(), "topic": topicProp()}
}
func newTopicProps() map[string]any {
	return map[string]any{"space": spaceProp(), "topic": newNameProp("Topic")}
}
func topicReq() []string { return []string{"space", "topic"} }
