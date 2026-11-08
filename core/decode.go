package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

var unknownField = regexp.MustCompile(`unknown field "([^"]+)"`)

type versionsKey struct{}

type versions struct{ service, caller string }

// WithVersions returns ctx carrying the service's version and the
// caller's (empty when the caller sent none), for Decode's hint on an
// unknown field.
func WithVersions(ctx context.Context, service, caller string) context.Context {
	return context.WithValue(ctx, versionsKey{}, versions{service, caller})
}

// Decode decodes a request's JSON strictly, as every front door must: an
// unknown field, a wrong type or trailing data is invalid, naming the
// field where there is one. An empty input decodes as {}.
//
// A top-level client_version string is the caller's version: Decode
// removes it before decoding, and it wins over the one in ctx. When ctx
// carries the service's version, an unknown field's message says whether
// a version difference is the likely cause and how to fix it.
func Decode(ctx context.Context, b []byte, v any) error {
	if len(bytes.TrimSpace(b)) == 0 {
		b = []byte("{}")
	}
	vs, _ := ctx.Value(versionsKey{}).(versions)
	b, vs.caller = takeClientVersion(b, vs.caller)
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		e := &Error{Code: Invalid, Message: "invalid JSON: " + err.Error()}
		if m := unknownField.FindStringSubmatch(err.Error()); m != nil {
			e.Field = m[1]
			e.Message += vs.hint()
		}
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) {
			e.Field = te.Field
		}
		return e
	}
	if dec.More() {
		return &Error{Code: Invalid, Message: "invalid JSON: trailing data"}
	}
	return nil
}

// takeClientVersion removes a top-level client_version string from the
// JSON object b, returning b without it and its value, or b unchanged
// and caller when there is none.
func takeClientVersion(b []byte, caller string) ([]byte, string) {
	var obj map[string]json.RawMessage
	if !bytes.Contains(b, []byte(`"client_version"`)) || json.Unmarshal(b, &obj) != nil {
		return b, caller
	}
	var cv string
	if raw, ok := obj["client_version"]; !ok || json.Unmarshal(raw, &cv) != nil {
		return b, caller
	}
	delete(obj, "client_version")
	out, err := json.Marshal(obj)
	if err != nil {
		return b, caller
	}
	return out, cv
}

// hint is what an unknown field's message gains: nothing without the
// service's version or when the caller's equals it.
func (vs versions) hint() string {
	switch {
	case vs.service == "" || vs.caller == vs.service:
		return ""
	case vs.caller == "":
		return fmt.Sprintf(" (the service is %s; if your client is newer, run `code-central service restart`)", vs.service)
	default:
		return fmt.Sprintf(": the caller is version %s and the service is %s; run `code-central service restart`", vs.caller, vs.service)
	}
}
