package mcp

// A tool derived from an operation's definition (ops.Definitions): its
// input schema comes from the input type by reflection and its
// description from the definition; a call dispatches the definition.

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fieldwalk"
	"github.com/tomoram/code-central/ops"
)

// derived are the tools derived from the definitions of the agent
// operations, by name. A definition whose input the derivation can't
// express fails here, at start-up, naming the operation and the field.
var derived = map[string]tool{}

func init() {
	for _, op := range ops.Definitions() {
		if op.Op().Audience != ops.AgentAndHuman {
			continue
		}
		t, err := derive(op)
		if err != nil {
			panic(fmt.Sprintf("mcp: %s: %v", op.Op().Name, err))
		}
		derived[op.Op().Name] = t
	}
}

// Descriptions maps each tool to its description, for core's context
// budget test.
func Descriptions() map[string]string {
	out := map[string]string{}
	for name, t := range derived {
		out[name] = t.description
	}
	return out
}

// derive is op's tool.
func derive(op ops.Operation) (tool, error) {
	s, err := schema(op.Input(), op.FieldDescription)
	if err != nil {
		return tool{}, err
	}
	props := s["properties"].(map[string]any)
	for name, p := range props {
		if values, ok := op.FieldEnum(name); ok {
			p.(map[string]any)["enum"] = values
		}
	}
	return tool{description: op.Description(), schema: s, op: op}, nil
}

var refType = reflect.TypeFor[core.Ref]()

// schema is the input schema of struct type t: an object with its JSON
// fields as properties and additionalProperties false. A field's
// description is its desc tag, unless describe overrides it by JSON name;
// required:"true" lists it in required, in field order; enum:"A,B" gives a
// string an explicit enum; minItems:"n" gives a list its minItems; mcp:"-"
// leaves it out. Embedded structs are
// flattened. A field with no description, or a shape the derivation can't
// express, is an error naming the field.
func schema(t reflect.Type, describe func(name string) (string, bool)) (map[string]any, error) {
	props := map[string]any{}
	var required []string
	for _, f := range fieldwalk.Fields(t) {
		if f.Tag.Get("mcp") == "-" {
			continue
		}
		desc, ok := "", false
		if describe != nil {
			desc, ok = describe(f.Name)
		}
		if !ok {
			desc = f.Tag.Get("desc")
		}
		if desc == "" && !isStruct(f.Type) {
			return nil, fmt.Errorf("field %s has no desc tag or description override", f.Name)
		}
		p, err := property(f.Type, desc, f.Tag.Get("enum"), f.Tag.Get("minItems"))
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", f.Name, err)
		}
		props[f.Name] = p
		if f.Tag.Get("required") == "true" {
			required = append(required, f.Name)
		}
	}
	return object(props, required...), nil
}

// isStruct reports whether t, or what it points to, is a struct: a
// nested object, which needs no description of its own.
func isStruct(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Kind() == reflect.Struct
}

// property is the schema of a field of type t with description desc.
func property(t reflect.Type, desc, enum, minItems string) (map[string]any, error) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if enum != "" && t.Kind() != reflect.String {
		return nil, fmt.Errorf("enum on a %s, not a string", t)
	}
	switch {
	case t == refType:
		return ref(desc), nil
	case t.Kind() == reflect.String:
		p := text(desc)
		if enum != "" {
			p["enum"] = strings.Split(enum, ",")
		}
		return p, nil
	case t.Kind() == reflect.Bool:
		return flag(desc), nil
	case t.Kind() == reflect.Struct:
		p, err := schema(t, nil)
		if err != nil {
			return nil, err
		}
		if desc != "" {
			p["description"] = desc
		}
		return p, nil
	case t.Kind() == reflect.Slice:
		items, err := item(t.Elem())
		if err != nil {
			return nil, err
		}
		p := map[string]any{"type": "array", "items": items, "description": desc}
		if minItems != "" {
			n, err := strconv.Atoi(minItems)
			if err != nil {
				return nil, fmt.Errorf("minItems %q is not a number", minItems)
			}
			p["minItems"] = n
		}
		return p, nil
	case t.Kind() == reflect.Map && t.Key().Kind() == reflect.String && t.Elem().Kind() == reflect.String:
		return tagFilter(desc), nil
	}
	return nil, fmt.Errorf("a %s has no schema", t)
}

// item is the schema of a list's element type t: a string, an ID or a
// nested object, whose fields carry their own tags.
func item(t reflect.Type) (map[string]any, error) {
	switch {
	case t == refType:
		return map[string]any{"type": []string{"string", "integer"}}, nil
	case t.Kind() == reflect.String:
		return map[string]any{"type": "string"}, nil
	case t.Kind() == reflect.Struct:
		return schema(t, nil)
	}
	return nil, fmt.Errorf("a list of %s has no schema", t)
}
