package main

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/tomoram/code-central/internal/fieldwalk"
	"github.com/tomoram/code-central/ops"
)

// emptyInput names the input of an operation that takes none.
const emptyInput = "EmptyInput"

// placeholder is a {name} in a path pattern.
var placeholder = regexp.MustCompile(`\{(\w+)\}`)

// forUI reports whether the UI may call an operation: the HTTP door
// serves it to the human.
func forUI(op ops.Operation) bool {
	a := op.Op().Audience
	return a == ops.AgentAndHuman || a == ops.HumanOnly
}

// entry is one operation of the table.
type entry struct {
	name, method, pattern string
	// bind are the input fields that fill the placeholders, as
	// placeholder and field, in pattern order.
	bind [][2]string
	// query are a GET's input fields, as field and query parameter, in
	// field order.
	query [][2]string
	input reflect.Type
}

// entries are the table's operations, in the order given, after failing
// on each one the UI can't express.
func (g *generator) entries(operations []ops.Operation) []entry {
	var out []entry
	for _, op := range operations {
		if !forUI(op) {
			continue
		}
		name := op.Op().Name
		method, pattern := op.Route()
		e := entry{name: name, method: method, pattern: pattern, input: op.Input()}
		bound := map[string]string{}
		for _, f := range fieldwalk.Fields(e.input) {
			if p := f.Tag.Get("path"); p != "" {
				bound[p] = f.Name
			}
		}
		for _, m := range placeholder.FindAllStringSubmatch(pattern, -1) {
			if m[1] == "key" {
				continue
			}
			field, ok := bound[m[1]]
			if !ok {
				g.fail("operation %s: the placeholder {%s} in %s has no input field bound to it", name, m[1], pattern)
				continue
			}
			e.bind = append(e.bind, [2]string{m[1], field})
		}
		if method == "GET" {
			for _, f := range fieldwalk.AllFields(e.input) {
				if param, ok := ops.QueryParam(f); ok && ops.QueryScalar(f.Type) {
					e.query = append(e.query, [2]string{inputName(f), param})
				}
			}
		}
		out = append(out, e)
	}
	return out
}

// inputName is the name a field has in a generated input type: its JSON
// name, or, for a json:"-" field a query carries, its query name.
func inputName(f fieldwalk.Field) string {
	if f.Hidden {
		return f.Tag.Get("query")
	}
	return f.Name
}

// reachInputs queues each input type the table reaches, once, and marks
// it as an input so that it is written by the inputs' rules. It returns
// the type name each entry's input has.
func (g *generator) reachInputs(entries []entry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		t := e.input
		if t.Kind() != reflect.Struct {
			g.fail("operation %s: input %s isn't a struct", e.name, t)
			continue
		}
		if t.Name() == "" {
			names[i] = emptyInput
			continue
		}
		names[i] = t.Name()
		if e.method == "GET" {
			g.getInputs[t] = true
		}
		if !g.ownJSON(t, "operation "+e.name) && g.claim(t) {
			g.inputs[t] = true
			g.queue = append(g.queue, t)
		}
	}
	return names
}

// inputIface is input struct t's interface: the fields a door accepts,
// embedded structs flattened. A field is required when it is tagged
// required or bound to a path placeholder, and optional otherwise; a
// json:"-" field a query carries is there under its query name; a query
// field of a GET that only the type's own EncodeQuery can encode, and a
// json:"-" field with no query tag, are left out.
func (g *generator) inputIface(t reflect.Type) string {
	src, err := g.source(t.PkgPath())
	if err != nil {
		g.fail("%s: %v", t, err)
		return ""
	}
	docs := src.fieldDocs(t.Name())
	var body strings.Builder
	for _, f := range fieldwalk.AllFields(t) {
		at := t.Name() + "." + f.StructField.Name
		_, query := ops.QueryParam(f)
		switch {
		case f.Hidden && !(query && ops.QueryScalar(f.Type)):
			continue
		case !f.Hidden && g.getInputs[t] && query && !ops.QueryScalar(f.Type):
			continue
		}
		mark := "?"
		if f.Tag.Get("path") != "" || f.Tag.Get("required") == "true" {
			mark = ""
		}
		typ := g.ts(f.Type, at)
		body.WriteString(comment("  ", docs[f.StructField.Name]))
		fmt.Fprintf(&body, "  %s%s: %s;\n", property(inputName(f)), mark, typ)
	}
	return comment("", src.typeDoc(t.Name())) + fmt.Sprintf("export interface %s {\n%s}\n", t.Name(), body.String())
}

// table is the Operation Table: each operation the UI may call, with its
// method, its path, the input fields that fill the path and the query,
// and its input type; and the type that maps each name to its input.
func table(entries []entry, inputs []string) string {
	var b strings.Builder
	b.WriteString("// OPERATIONS are the operations the UI may call, in the order ops lists\n")
	b.WriteString("// them: each one's method, its path pattern, the input fields that fill\n")
	b.WriteString("// the path's placeholders (bind) and, for a GET, go out as query\n")
	b.WriteString("// parameters (query), and the name of its input type.\n")
	b.WriteString("export const OPERATIONS = {\n")
	for i, e := range entries {
		fmt.Fprintf(&b, "  %s: {\n    method: %s,\n    path: %s,\n    bind: %s,\n    query: %s,\n    input: %s,\n  },\n",
			property(e.name), quote(e.method), quote(e.pattern), object(e.bind), object(e.query), quote(inputs[i]))
	}
	b.WriteString("} as const;\n")
	b.WriteString("export type OperationName = keyof typeof OPERATIONS;\n\n")
	b.WriteString("// OperationInputs maps each operation to its input type.\n")
	b.WriteString("export interface OperationInputs {\n")
	for i, e := range entries {
		fmt.Fprintf(&b, "  %s: %s;\n", property(e.name), inputs[i])
	}
	b.WriteString("}\n")
	return b.String()
}

// object is pairs as a TypeScript object literal.
func object(pairs [][2]string) string {
	if len(pairs) == 0 {
		return "{}"
	}
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = property(p[0]) + ": " + quote(p[1])
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}
