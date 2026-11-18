package ops

import (
	"reflect"

	"github.com/tomoram/code-central/internal/fieldwalk"
)

// QueryParam is the GET query parameter that field f stands for, so that
// the HTTP door decodes a query and the client encodes one by the same
// rule: the field's query tag, else its JSON name. The path-bound field
// and a json:"-" field with no query tag are no parameter.
func QueryParam(f fieldwalk.Field) (string, bool) {
	if f.Tag.Get("path") != "" {
		return "", false
	}
	if name := f.Tag.Get("query"); name != "" {
		return name, true
	}
	if f.Hidden {
		return "", false
	}
	return f.Name, true
}

// QueryScalar reports whether the tags alone carry a field of type t
// across a query: a string, a bool or a *bool, or a list of strings,
// including typed strings such as a State or a Ref. Any other type needs
// the input type's own query methods.
func QueryScalar(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.String, reflect.Bool:
		return true
	case reflect.Pointer:
		return t.Elem().Kind() == reflect.Bool
	case reflect.Slice:
		return t.Elem().Kind() == reflect.String
	}
	return false
}
