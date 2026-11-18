// Package fieldwalk reads a struct's fields as encoding/json names them,
// so that every reader of a Go type's wire shape works a field's JSON name
// out the same way: the Core's acknowledgements and strict decoding, the
// MCP door's schema derivation and the UI's generated wire types
// (internal/wiretypes). It uses only the standard library.
package fieldwalk

import (
	"reflect"
	"slices"
	"strings"
)

// Info is how encoding/json treats one struct field.
type Info struct {
	// Name is the field's JSON name: the json tag's name, else the Go name.
	Name string
	// Options are the json tag's options after the name, such as omitempty.
	Options []string
	// OmitEmpty is set by the omitempty or omitzero option.
	OmitEmpty bool
	// Skipped fields are not in the JSON: json:"-", or unexported and not
	// an embedded struct.
	Skipped bool
	// Flattened fields are embedded structs with no JSON name of their
	// own: their fields take their place in the JSON object.
	Flattened bool
}

// Of reads one struct field.
func Of(f reflect.StructField) Info {
	name, rest, _ := strings.Cut(f.Tag.Get("json"), ",")
	var opts []string
	if rest != "" {
		opts = strings.Split(rest, ",")
	}
	info := Info{Name: name, Options: opts, OmitEmpty: slices.Contains(opts, "omitempty") || slices.Contains(opts, "omitzero")}
	embedded := f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct
	switch {
	case name == "-", !f.IsExported() && !embedded:
		info.Skipped = true
	case embedded:
		info.Flattened = true
	case name == "":
		info.Name = f.Name
	}
	return info
}

// Field is one JSON-visible field of a walked struct.
type Field struct {
	reflect.StructField
	// Name is the field's JSON name.
	Name string
	// OmitEmpty is set by the omitempty or omitzero option.
	OmitEmpty bool
	// Index addresses the field from the walked struct, through any
	// embedded structs, for reflect.Value.FieldByIndex.
	Index []int
}

// Fields are struct type t's JSON-visible fields in declaration order,
// with each embedded struct's fields in its place. A pointer to a struct
// is walked as the struct; any other type has no fields.
func Fields(t reflect.Type) []Field {
	return fields(t, nil)
}

func fields(t reflect.Type, prefix []int) []Field {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	var out []Field
	for f := range t.Fields() {
		info := Of(f)
		index := append(slices.Clone(prefix), f.Index...)
		switch {
		case info.Skipped:
		case info.Flattened:
			out = append(out, fields(f.Type, index)...)
		default:
			out = append(out, Field{StructField: f, Name: info.Name, OmitEmpty: info.OmitEmpty, Index: index})
		}
	}
	return out
}

// Names are the JSON names of Fields(t), in that order.
func Names(t reflect.Type) []string {
	var out []string
	for _, f := range Fields(t) {
		out = append(out, f.Name)
	}
	return out
}
