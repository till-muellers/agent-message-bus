package tools

import (
	"bytes"
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

// requireExactKeys fails with invalid "invalid arguments: json: unknown field …" for the first
// key of object (sorted) that is not exactly the JSON name of a field of struct st. Other
// types, and input that is no JSON object, pass: the decode into st reports those.
func requireExactKeys(st reflect.Type, object []byte) error {
	if st.Kind() != reflect.Struct {
		return nil
	}
	var fields map[string]json.RawMessage
	if json.NewDecoder(bytes.NewReader(object)).Decode(&fields) != nil {
		return nil
	}
	names := jsonFields(st)
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		if _, found := names[key]; !found {
			return apperr.New(apperr.Invalid, "invalid arguments: json: unknown field %q", key)
		}
	}
	return nil
}

// jsonFields maps the JSON name of each exported field of st (the tag name, else the field
// name) to the field.
func jsonFields(st reflect.Type) map[string]reflect.StructField {
	fields := make(map[string]reflect.StructField, st.NumField())
	for i := range st.NumField() {
		f := st.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if !f.IsExported() || name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		fields[name] = f
	}
	return fields
}
