package tools

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

// requireExactKeys fails with invalid "invalid arguments: json: unknown field …" for the first
// key of object (sorted) that is not exactly the JSON name of a field of struct st; other
// types pass unchecked.
func requireExactKeys(st reflect.Type, object []byte) error {
	if st.Kind() != reflect.Struct {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(object, &fields); err != nil {
		return apperr.New(apperr.Invalid, "invalid arguments: %v", err)
	}
	names := jsonNames(st)
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		if !names[key] {
			return apperr.New(apperr.Invalid, "invalid arguments: json: unknown field %q", key)
		}
	}
	return nil
}

// jsonNames is the set of JSON names of st's exported fields: the tag name, else the field name.
func jsonNames(st reflect.Type) map[string]bool {
	names := make(map[string]bool, st.NumField())
	for i := range st.NumField() {
		f := st.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if !f.IsExported() || name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		names[name] = true
	}
	return names
}
