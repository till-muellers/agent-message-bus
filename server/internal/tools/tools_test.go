package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

type sample struct {
	Space string `json:"space"`
	N     int64  `json:"n"`
}

func TestDecode(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    sample
		invalid bool
	}{
		{"empty", "", sample{}, false},
		{"null", "null", sample{}, false},
		{"fields", `{"space":"a","n":3}`, sample{Space: "a", N: 3}, false},
		{"unknown field", `{"x":1}`, sample{}, true},
		{"wrong type", `{"n":"x"}`, sample{}, true},
		{"malformed", `{`, sample{}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Decode[sample](json.RawMessage(c.in))
			if c.invalid {
				if k, _ := apperr.KindOf(err); k != apperr.Invalid {
					t.Fatalf("want invalid, got %v", err)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
}

// encoding/json matches keys case-insensitively; the closed schema names each key exactly.
func TestDecodeRejectsCaseMismatchedKeys(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, in, want string }{
		{"capitalised", `{"Space":"a"}`, `invalid arguments: json: unknown field "Space"`},
		{"upper case", `{"space":"a","N":3}`, `invalid arguments: json: unknown field "N"`},
		{"duplicate in other case", `{"n":1,"N":3}`, `invalid arguments: json: unknown field "N"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := Decode[sample](json.RawMessage(c.in))
			if k, _ := apperr.KindOf(err); k != apperr.Invalid || err.Error() != c.want {
				t.Fatalf("got %v, want invalid %q", err, c.want)
			}
		})
	}
}

func TestDefineRequiresFields(t *testing.T) {
	tl := define("t", "a", "d", "r", []string{"space", "n"}, map[string]any{"space": str("s"), "n": integer("n")},
		func(_ context.Context, _ auth.Principal, a sample) (any, error) { return a, nil })
	cases := []struct {
		name, in, msg string
	}{
		{"no args", "", "space is required"},
		{"missing n", `{"space":"a"}`, "n is required"},
		{"null space", `{"space":null,"n":1}`, "space is required"},
		{"ok", `{"space":"a","n":1}`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := tl.Call(context.Background(), auth.Principal{}, json.RawMessage(c.in))
			if c.msg == "" {
				if err != nil || got != (sample{Space: "a", N: 1}) {
					t.Fatalf("got %+v, %v", got, err)
				}
				return
			}
			if k, _ := apperr.KindOf(err); k != apperr.Invalid || err.Error() != c.msg {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestListNeverNil(t *testing.T) {
	got, err := list[string](nil, nil)
	b, _ := json.Marshal(got)
	if err != nil || string(b) != "[]" {
		t.Fatalf("got %s, %v", b, err)
	}
}
