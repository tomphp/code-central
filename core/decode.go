package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
)

var unknownField = regexp.MustCompile(`unknown field "([^"]+)"`)

// Decode decodes a request's JSON strictly, as every front door must: an
// unknown field, a wrong type or trailing data is invalid, naming the
// field where there is one. An empty input decodes as {}.
func Decode(b []byte, v any) error {
	if len(bytes.TrimSpace(b)) == 0 {
		b = []byte("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		e := &Error{Code: Invalid, Message: "invalid JSON: " + err.Error()}
		if m := unknownField.FindStringSubmatch(err.Error()); m != nil {
			e.Field = m[1]
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
