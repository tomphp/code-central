package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

var unknownField = regexp.MustCompile(`unknown field "([^"]+)"`)

type versionsKey struct{}

type versions struct{ service, client string }

// WithVersions returns ctx carrying the service's version and the
// client's (empty when the client sent none), for Decode's hint on an
// unknown field.
func WithVersions(ctx context.Context, service, client string) context.Context {
	return context.WithValue(ctx, versionsKey{}, versions{service, client})
}

// Decode decodes a request's JSON strictly, as every front door must: an
// unknown field, a wrong type or trailing data is invalid, naming the
// field where there is one. An empty input decodes as {}.
//
// A top-level client_version string is the client's version: Decode
// removes it before decoding, and it wins over the one in ctx. When ctx
// carries the service's version, an unknown field's message says whether
// a version difference is the likely cause and how to fix it.
func Decode(ctx context.Context, b []byte, v any) error {
	if len(bytes.TrimSpace(b)) == 0 {
		b = []byte("{}")
	}
	vs, _ := ctx.Value(versionsKey{}).(versions)
	b, vs.client = takeClientVersion(b, vs.client)
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		e := &Error{Code: Invalid, Message: "invalid JSON: " + err.Error()}
		if m := unknownField.FindStringSubmatch(err.Error()); m != nil {
			e.Field = m[1]
			e.Message += acceptedFields(v, m[1]) + vs.hint()
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
// and client when there is none.
func takeClientVersion(b []byte, client string) ([]byte, string) {
	var obj map[string]json.RawMessage
	if !bytes.Contains(b, []byte(`"client_version"`)) || json.Unmarshal(b, &obj) != nil {
		return b, client
	}
	var cv string
	if raw, ok := obj["client_version"]; !ok || json.Unmarshal(raw, &cv) != nil {
		return b, client
	}
	delete(obj, "client_version")
	out, err := json.Marshal(obj)
	if err != nil {
		return b, client
	}
	return out, cv
}

// hint is what an unknown field's message gains: nothing without the
// service's version or when the client's equals it.
func (vs versions) hint() string {
	switch {
	case vs.service == "" || vs.client == vs.service:
		return ""
	case vs.client == "":
		return fmt.Sprintf(" (the service is %s; if your client is newer, run `code-central service restart`)", vs.service)
	default:
		return fmt.Sprintf(": the caller is version %s and the service is %s; run `code-central service restart`", vs.client, vs.service)
	}
}

// acceptedFields is what an unknown field's message gains: the fields v
// accepts, and the likeliest one the caller meant, when there is one.
func acceptedFields(v any, unknown string) string {
	names := jsonFields(reflect.TypeOf(v))
	if len(names) == 0 {
		return ""
	}
	msg := ": accepted fields are " + strings.Join(names, ", ")
	if m := likelyMatch(names, unknown); m != "" {
		msg += fmt.Sprintf("; did you mean `%s`?", m)
	}
	return msg
}

// jsonFields lists the JSON names of the struct t (or what it points to),
// in declaration order, including those of embedded structs.
func jsonFields(t reflect.Type) []string {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	var names []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		switch {
		case name == "-":
		case name != "":
			names = append(names, name)
		case f.Anonymous:
			names = append(names, jsonFields(f.Type)...)
		case f.IsExported():
			names = append(names, f.Name)
		}
	}
	return names
}

// likelyMatch is the name closest to unknown: one that contains it or is
// contained in it, or within two edits of it; empty when none is.
func likelyMatch(names []string, unknown string) string {
	best, bestDist := "", 3
	for _, n := range names {
		d := editDistance(n, unknown)
		if len(n) > 2 && len(unknown) > 2 && (strings.Contains(unknown, n) || strings.Contains(n, unknown)) {
			d = 0
		}
		if d < bestDist {
			best, bestDist = n, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
