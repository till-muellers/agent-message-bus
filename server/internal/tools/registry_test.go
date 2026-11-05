package tools

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

func TestRegistryHasAllToolsSortedAndUnique(t *testing.T) {
	ts := New(Deps{}).list
	if len(ts) != 34 {
		t.Fatalf("got %d tools, want 34", len(ts))
	}
	names := make([]string, len(ts))
	seen := map[string]bool{}
	for i, tl := range ts {
		names[i] = tl.Name
		if seen[tl.Name] {
			t.Fatalf("duplicate tool %s", tl.Name)
		}
		seen[tl.Name] = true
		if tl.Description == "" || tl.Result == "" || tl.Call == nil {
			t.Fatalf("tool %s missing description, result or call", tl.Name)
		}
		if !slices.Contains(groupOrder(), tl.Group) {
			t.Fatalf("tool %s has unknown group %q", tl.Name, tl.Group)
		}
	}
	if !sort.StringsAreSorted(names) {
		t.Fatalf("names not sorted: %v", names)
	}
}

func TestRegistryCall(t *testing.T) {
	t.Parallel()
	echo := Tool{Meta: Meta{Name: "echo"}, Call: func(_ context.Context, _ auth.Principal, args json.RawMessage) (any, error) {
		return string(args), nil
	}}
	reg := NewRegistry([]Tool{echo})
	got, err := reg.Call(context.Background(), auth.Principal{}, "echo", json.RawMessage(`{"a":1}`))
	if err != nil || got != `{"a":1}` {
		t.Fatalf("echo = %v, %v", got, err)
	}
	_, err = reg.Call(context.Background(), auth.Principal{}, "bus_nope", nil)
	if k, _ := apperr.KindOf(err); k != apperr.NotFound || err.Error() != "tool bus_nope not found" {
		t.Fatalf("unknown tool = %v", err)
	}
}

func TestSchemasUseAllowedSubset(t *testing.T) {
	for _, tl := range Metadata() {
		t.Run(tl.Name, func(t *testing.T) {
			if tl.Schema["additionalProperties"] != false {
				t.Fatalf("additionalProperties must be false")
			}
			checkSchema(t, tl.Schema)
		})
	}
}

func allowedTypes() map[any]bool {
	return map[any]bool{"object": true, "string": true, "integer": true, "boolean": true}
}

func allowedKeys() map[string]bool {
	return map[string]bool{"type": true, "description": true, "properties": true, "required": true, "additionalProperties": true}
}

func checkSchema(t *testing.T, s map[string]any) {
	t.Helper()
	if !allowedTypes()[s["type"]] {
		t.Fatalf("disallowed type %v", s["type"])
	}
	for k := range s {
		if !allowedKeys()[k] {
			t.Fatalf("disallowed key %s", k)
		}
	}
	if s["type"] == "object" && s["additionalProperties"] != false {
		t.Fatalf("nested object must set additionalProperties:false")
	}
	props, _ := s["properties"].(map[string]any)
	for _, p := range props {
		checkSchema(t, p.(map[string]any))
	}
	req, _ := s["required"].([]string)
	for _, r := range req {
		if _, found := props[r]; !found {
			t.Fatalf("required %s not in properties", r)
		}
	}
}
