package tools

import (
	"fmt"

	"github.com/till-muellers/agent-message-bus/server/internal/domain"
)

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

// nameDesc describes a name argument with the domain name rule.
func nameDesc(what string) string {
	return fmt.Sprintf("%s name, matching %s.", what, domain.NamePattern)
}

func spaceProp() map[string]any  { return str(nameDesc("Space")) }
func topicProp() map[string]any  { return str(nameDesc("Topic")) }
func boardProp() map[string]any  { return str(nameDesc("Blackboard")) }
func threadProp() map[string]any { return integer("Thread id.") }

func bodyProp() map[string]any {
	return str(fmt.Sprintf("Message body, 1..%d bytes.", domain.MaxBodyBytes))
}

func titleProp() map[string]any {
	return str(fmt.Sprintf("Thread title, 1..%d characters.", domain.MaxTitleChars))
}

func criticalProp() map[string]any {
	return boolean("Leaders and admins only. omp recipients are interrupted (steer); other clients see critical: true.")
}
