package inbox

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestClassOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		typ      Type
		critical bool
		want     Class
	}{
		{DirectMessage, false, ClassQueue},
		{DirectMessage, true, ClassInterrupt},
		{TopicMessage, false, ClassQueue},
		{TopicMessage, true, ClassInterrupt},
		{ThreadPost, false, ClassQueue},
		{ThreadPost, true, ClassInterrupt},
		{TopicClosed, false, ClassDeferred},
		{TopicClosed, true, ClassInterrupt},
		{ThreadRemoved, false, ClassDeferred},
		{ThreadRemoved, true, ClassInterrupt},
		{BlackboardRemoved, false, ClassDeferred},
		{BlackboardRemoved, true, ClassInterrupt},
	}
	for _, tc := range tests {
		if got := ClassOf(tc.typ, tc.critical); got != tc.want {
			t.Errorf("ClassOf(%s, %v) = %q, want %q", tc.typ, tc.critical, got, tc.want)
		}
	}
}

// TestEveryTypeHasClass fails when a Type constant in entry.go has no row in classByType.
func TestEveryTypeHasClass(t *testing.T) {
	t.Parallel()
	f, err := parser.ParseFile(token.NewFileSet(), "entry.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing entry.go: %v", err)
	}
	types := typeConstants(f)
	if len(types) == 0 {
		t.Fatal("no Type constants found in entry.go")
	}
	for _, name := range types {
		if _, ok := classByType[Type(name)]; !ok {
			t.Errorf("entry type %q has no class in classByType", name)
		}
	}
}

// typeConstants returns the string values of the constants declared with type Type.
func typeConstants(f *ast.File) []string {
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if id, ok := spec.Type.(*ast.Ident); !ok || id.Name != "Type" {
			return true
		}
		for _, v := range spec.Values {
			if lit, ok := v.(*ast.BasicLit); ok {
				out = append(out, strings.Trim(lit.Value, `"`))
			}
		}
		return true
	})
	return out
}

func TestEntriesCarryClass(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	critical := w.entry("worker", "alpha", "urgent")
	critical.Critical = true
	closed := w.entry("worker", "alpha", "")
	closed.Type, closed.Topic = TopicClosed, "ops"
	stored, err := w.svc.insert(t.Context(), w.db, []Entry{critical, closed})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if stored[0].Class != ClassInterrupt || stored[1].Class != ClassDeferred {
		t.Fatalf("inserted classes = %q, %q", stored[0].Class, stored[1].Class)
	}
	read, err := w.svc.Read(t.Context(), w.principals["memberA"], 0, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	stream := startStream(t.Context(), t, w, "memberA")
	streamed := make([]Entry, len(read))
	for i := range streamed {
		streamed[i] = stream.nextEntry()
	}
	want := map[string]Class{"a1": ClassQueue, "a2": ClassQueue, "urgent": ClassInterrupt, "": ClassDeferred}
	for path, es := range map[string][]Entry{"bus_inbox_read": read, "GET /v1/inbox/stream": streamed} {
		for _, e := range es {
			if e.Class != want[e.Body] {
				t.Errorf("%s: entry %q class = %q, want %q", path, e.Body, e.Class, want[e.Body])
			}
		}
	}
	assertClassKey(t, read[0])
}

func assertClassKey(t *testing.T, e Entry) {
	t.Helper()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"class":"queue"`) {
		t.Fatalf("entry JSON %s lacks \"class\":\"queue\"", raw)
	}
}
