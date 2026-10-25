package tools

import (
	"sort"
	"testing"
)

func TestRegistryHasAllToolsSortedAndUnique(t *testing.T) {
	ts := New(Deps{}).List()
	if len(ts) != 29 {
		t.Fatalf("got %d tools, want 29", len(ts))
	}
	names := make([]string, len(ts))
	seen := map[string]bool{}
	for i, tl := range ts {
		names[i] = tl.Name
		if seen[tl.Name] {
			t.Fatalf("duplicate tool %s", tl.Name)
		}
		seen[tl.Name] = true
		if tl.Description == "" || tl.Call == nil {
			t.Fatalf("tool %s missing description or call", tl.Name)
		}
	}
	if !sort.StringsAreSorted(names) {
		t.Fatalf("names not sorted: %v", names)
	}
}

func TestSchemasUseAllowedSubset(t *testing.T) {
	for _, tl := range New(Deps{}).List() {
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
