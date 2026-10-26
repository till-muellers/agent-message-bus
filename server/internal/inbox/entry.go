package inbox

// Type is the kind of inbox entry.
type Type string

const (
	DirectMessage     Type = "direct_message"
	TopicMessage      Type = "topic_message"
	TopicClosed       Type = "topic_closed"
	ThreadPost        Type = "thread_post"
	ThreadRemoved     Type = "thread_removed"
	BlackboardRemoved Type = "blackboard_removed"
)

// Entry is one item in an agent's inbox. Producers set Space before publishing.
type Entry struct {
	ID          int64  `json:"id"`
	AgentID     int64  `json:"-"`
	SpaceID     int64  `json:"-"`
	Space       string `json:"space"`
	Type        Type   `json:"type"`
	From        string `json:"from"`
	Critical    bool   `json:"critical"`
	Class       Class  `json:"class"` // derived by ClassOf from Type and Critical, never set by producers
	Topic       string `json:"topic,omitempty"`
	Blackboard  string `json:"blackboard,omitempty"`
	ThreadID    int64  `json:"threadId,omitempty"`
	ThreadTitle string `json:"threadTitle,omitempty"`
	Body        string `json:"body,omitempty"`
	CreatedAt   string `json:"createdAt"`
}
