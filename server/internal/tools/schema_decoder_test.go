package tools

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestSchemaMatchesDecoder keeps the JSON Schema an agent sees and the argument struct
// its Call decodes into in step: same property names, same JSON types, and no required
// property decoding into a pointer (a pointer marks an argument whose absence matters).
func TestSchemaMatchesDecoder(t *testing.T) {
	t.Parallel()
	for _, tl := range New(Deps{}).List() {
		t.Run(tl.Name, func(t *testing.T) {
			if tl.argsType == nil {
				t.Fatal("argument type unknown: build the tool with define")
			}
			fields := jsonFields(tl.argsType)
			props, _ := tl.Schema["properties"].(map[string]any)
			checkFieldsInSchema(t, fields, props)
			checkSchemaInFields(t, fields, props)
			checkRequiredNotPointer(t, fields, tl.Schema["required"].([]string))
		})
	}
}

func checkFieldsInSchema(t *testing.T, fields map[string]reflect.Type, props map[string]any) {
	t.Helper()
	for name, ft := range fields {
		prop, found := props[name].(map[string]any)
		if !found {
			t.Errorf("decoder field %q missing from schema properties", name)
			continue
		}
		if want := schemaType(ft); prop["type"] != want {
			t.Errorf("property %q has schema type %v, decoder type %s wants %q", name, prop["type"], ft, want)
		}
	}
}

func checkSchemaInFields(t *testing.T, fields map[string]reflect.Type, props map[string]any) {
	t.Helper()
	for name := range props {
		if _, found := fields[name]; !found {
			t.Errorf("schema property %q has no decoder field (the decoder rejects it)", name)
		}
	}
}

func checkRequiredNotPointer(t *testing.T, fields map[string]reflect.Type, required []string) {
	t.Helper()
	for _, name := range required {
		if ft, found := fields[name]; found && ft.Kind() == reflect.Pointer {
			t.Errorf("required property %q decodes into pointer %s", name, ft)
		}
	}
}

// jsonFields maps the JSON names encoding/json decodes into the exported fields of struct st.
func jsonFields(st reflect.Type) map[string]reflect.Type {
	fields := map[string]reflect.Type{}
	for i := range st.NumField() {
		f := st.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if !f.IsExported() || name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		fields[name] = f.Type
	}
	return fields
}

// schemaType is the JSON Schema type a value decoding into ft must have.
func schemaType(ft reflect.Type) string {
	if ft.Kind() == reflect.Pointer {
		ft = ft.Elem()
	}
	ints := []reflect.Kind{reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64}
	switch {
	case ft.Kind() == reflect.String:
		return "string"
	case ft.Kind() == reflect.Bool:
		return "boolean"
	case slices.Contains(ints, ft.Kind()):
		return "integer"
	}
	return ft.Kind().String()
}
