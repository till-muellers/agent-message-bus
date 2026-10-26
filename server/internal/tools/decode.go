package tools

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

// decode strictly unmarshals args into T; empty or null args yield the zero value.
func decode[T any](args json.RawMessage) (T, error) {
	var v T
	trimmed := bytes.TrimSpace(args)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return v, nil
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return v, apperr.New(apperr.Invalid, "invalid arguments: %v", err)
	}
	return v, nil
}

// requirePresent fails with Invalid when any required field is absent or null.
func requirePresent(args json.RawMessage, required []string) error {
	if len(required) == 0 {
		return nil
	}
	fields, err := decode[map[string]json.RawMessage](args)
	if err != nil {
		return err
	}
	for _, name := range required {
		if v, found := fields[name]; !found || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return apperr.New(apperr.Invalid, "%s is required", name)
		}
	}
	return nil
}

// define builds a Tool whose arguments decode into A after checking required fields.
// desc states the purpose first, then the pitfalls; the Description served to agents is desc
// followed by the access sentence. result sketches the success result shape for the tool ledger.
func define[A any](name, access, desc, result string, required []string, props map[string]any,
	call func(ctx context.Context, p auth.Principal, a A) (any, error)) Tool {
	return Tool{
		Name:        name,
		Description: desc + " " + access,
		Access:      access,
		Result:      result,
		Schema:      object(required, props),
		Call: func(ctx context.Context, p auth.Principal, args json.RawMessage) (any, error) {
			a, err := decode[A](args)
			if err != nil {
				return nil, err
			}
			if err := requirePresent(args, required); err != nil {
				return nil, err
			}
			return call(ctx, p, a)
		},
	}
}

// list returns a non-nil slice so list results encode as JSON arrays.
func list[T any](v []T, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	if v == nil {
		v = []T{}
	}
	return v, nil
}

// okResult is the ledger shape of done's success result.
const okResult = "{ok: true}"

// done maps a plain error result to {ok:true}.
func done(err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

// receiptsResult is the ledger shape of sent's success result.
const receiptsResult = "{ok: true, receipts: [{to, delivery: live|queued}]}"

// sent maps a send's receipts to {ok:true, receipts}.
func sent(rs []inbox.Receipt, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "receipts": rs}, nil
}
