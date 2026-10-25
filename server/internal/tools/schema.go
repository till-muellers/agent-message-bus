package tools

// object builds a closed JSON Schema object with the given required fields and properties.
func object(required []string, props map[string]any) map[string]any {
	if required == nil {
		required = []string{}
	}
	if props == nil {
		props = map[string]any{}
	}
	return map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	}
}

func str(desc string) map[string]any     { return typed("string", desc) }
func integer(desc string) map[string]any { return typed("integer", desc) }
func boolean(desc string) map[string]any { return typed("boolean", desc) }

func typed(kind, desc string) map[string]any {
	return map[string]any{"type": kind, "description": desc}
}

// Shared property schemas.
var (
	spaceProp    = str("Space name.")
	topicProp    = str("Topic name.")
	boardProp    = str("Blackboard name.")
	threadProp   = integer("Thread id.")
	bodyProp     = str("Message body, 1..65536 bytes.")
	criticalProp = boolean("Leaders only: mark the message critical so it interrupts the recipient.")
)
