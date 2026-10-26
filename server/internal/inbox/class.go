package inbox

// Class tells clients how urgently an entry should reach the agent (contracts.md §4.1).
type Class string

const (
	// ClassInterrupt reaches the agent now, interrupting its running turn.
	ClassInterrupt Class = "interrupt"
	// ClassQueue reaches the agent at its next step and wakes an idle agent.
	ClassQueue Class = "queue"
	// ClassDeferred reaches the agent with its next turn and never wakes an idle agent.
	ClassDeferred Class = "deferred"
)

// classByType is the class of every non-critical entry type; a new Type adds its row here.
var classByType = map[Type]Class{
	DirectMessage:     ClassQueue,
	TopicMessage:      ClassQueue,
	ThreadPost:        ClassQueue,
	TopicClosed:       ClassDeferred,
	ThreadRemoved:     ClassDeferred,
	BlackboardRemoved: ClassDeferred,
}

// ClassOf is the only place that decides an entry's class: critical entries interrupt,
// all others take the class of their type.
func ClassOf(t Type, critical bool) Class {
	if critical {
		return ClassInterrupt
	}
	return classByType[t]
}
