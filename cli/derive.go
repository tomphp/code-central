package cli

import (
	"context"
	"flag"
	"fmt"
	"reflect"
	"strings"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/internal/fieldwalk"
	"github.com/tomoram/code-central/ops"
)

// Commands derived from an operation's definition (Spec code-central-928).
// The input type's cli tags give each field's place on the command line:
//
//	cli:"<id>"            a positional argument; the first is the Spec or
//	                      Ticket ID
//	cli:"<label>..."      the remaining positional arguments, as a list
//	cli:"--commit <sha>"  a flag, named by its first word
//
// A bool with no cli tag is the flag of its JSON name, such as --full.
// Positional arguments are required, a list at least one. A flag the
// required tag marks is bare in the usage line, any other is in brackets,
// and a list or map flag repeats. A map is <key>=<value> pairs.
// A command whose shape the tags can't give (--file or stdin long input, a
// default from the environment, more than one call) stays hand-written,
// with its operation and help from the definition through handWritten.

// cliField is one field of a derived command's input.
type cliField struct {
	index []int
	typ   reflect.Type
	// flag is the flag's name, "" for a positional argument.
	flag string
	// usage is the field's part of the usage line.
	usage    string
	variadic bool
}

// derive is the command for def, whose flags, positional arguments and
// usage line follow from its input type's cli tags. call is def's client
// method, such as (*client.Client).Approve; note, when set, follows the
// usage line. A field the CLI can't take panics at start-up, naming the
// operation and the field.
func derive[In, Out any](def *ops.Definition[In, Out], call func(*client.Client, context.Context, string, In) (Out, error), note string) command {
	name := strings.ReplaceAll(def.Name, "_", "-")
	fields := cliFields(def.Name, reflect.TypeFor[In]())
	usage := []string{name}
	for _, f := range fields {
		usage = append(usage, f.usage)
	}
	if note != "" {
		usage = append(usage, " "+note)
	}
	run := func(ctx context.Context, e Env, args []string) error {
		fs := flagSet(name)
		values := map[string]any{}
		for _, f := range fields {
			switch {
			case f.flag == "":
			case f.typ.Kind() == reflect.Bool:
				values[f.flag] = fs.Bool(f.flag, false, "")
			case f.typ.Kind() == reflect.String:
				values[f.flag] = fs.String(f.flag, "", "")
			default:
				var s strs
				fs.Var(&s, f.flag, "")
				values[f.flag] = &s
			}
		}
		sc, pos, err := derivedArgs(e, fs, fields, args)
		if err != nil {
			return err
		}
		var in In
		v := reflect.ValueOf(&in).Elem()
		for _, f := range fields {
			var vals []string
			switch p := values[f.flag].(type) {
			case *bool:
				v.FieldByIndex(f.index).SetBool(*p)
				continue
			case *string:
				vals = []string{*p}
			case *strs:
				vals = *p
			default:
				if vals, pos = pos[:1], pos[1:]; f.variadic {
					vals, pos = append(vals, pos...), nil
				}
			}
			if err := setField(v.FieldByIndex(f.index), vals); err != nil {
				return err
			}
		}
		out, err := call(sc.client, ctx, sc.project, in)
		if err != nil {
			return err
		}
		return printJSON(e, out)
	}
	return command{op: def.Name, help: def.Doc, usage: strings.Join(usage, " "), run: run}
}

// handWritten is the command for an operation whose shape the cli tags
// can't give: its usage and run are the CLI's own, and its operation and
// help come from def.
func handWritten(def ops.Operation, usage string, run func(ctx context.Context, e Env, args []string) error) command {
	return command{op: def.Op().Name, help: def.Description(), usage: usage, run: run}
}

// cliFields reads t's cli tags for operation op.
func cliFields(op string, t reflect.Type) []cliField {
	var out []cliField
	positional, variadic := 0, false
	for _, f := range fieldwalk.Fields(t) {
		tag := f.Tag.Get("cli")
		cf := cliField{index: f.Index, typ: f.Type}
		fail := func(why string) {
			panic(fmt.Sprintf("cli: %s: field %s %s", op, f.StructField.Name, why))
		}
		switch {
		case strings.HasPrefix(tag, "<"):
			cf.usage = tag
			_, cf.variadic = strings.CutSuffix(tag, "...")
			switch {
			case variadic:
				fail("follows a list of positional arguments, which must come last")
			case positional == 0 && cf.variadic:
				fail("is a list, but the first positional argument is the Spec or Ticket ID")
			case f.Type.Kind() == reflect.Bool, cf.variadic != (f.Type.Kind() == reflect.Slice || f.Type.Kind() == reflect.Map):
				fail(fmt.Sprintf("is a %s, which the positional argument %s can't take", f.Type, tag))
			}
			positional++
			variadic = cf.variadic
		case strings.HasPrefix(tag, "--"):
			cf.flag, _, _ = strings.Cut(strings.TrimPrefix(tag, "--"), " ")
			cf.usage = tag
		case tag == "" && f.Type.Kind() == reflect.Bool:
			cf.flag = strings.ReplaceAll(f.Name, "_", "-")
			cf.usage = "--" + cf.flag
		default:
			fail("has no cli tag; give it one, or hand-write the command")
		}
		if !cliType(f.Type) {
			fail(fmt.Sprintf("is a %s, which no cli tag takes", f.Type))
		}
		if cf.flag != "" {
			if f.Tag.Get("required") != "true" {
				cf.usage = "[" + cf.usage + "]"
			}
			if k := f.Type.Kind(); k == reflect.Slice || k == reflect.Map {
				cf.usage += "..."
			}
		}
		out = append(out, cf)
	}
	return out
}

// cliType reports whether a derived command can fill a field of type t: a
// string, a bool, a list of strings or a map of strings, including typed
// strings such as a Ref.
func cliType(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.String, reflect.Bool:
		return true
	case reflect.Slice:
		return t.Elem().Kind() == reflect.String
	case reflect.Map:
		return t.Key().Kind() == reflect.String && t.Elem().Kind() == reflect.String
	}
	return false
}

// derivedArgs parses args, resolves the scope and returns the positional
// arguments in fields' order, for the run to take one per positional
// field and the rest for a list. A command with positional arguments is
// an item command, with the Spec or Ticket ID first; one without takes
// none.
func derivedArgs(e Env, fs *flag.FlagSet, fields []cliField, args []string) (scope, []string, error) {
	rest, variadic := -1, false
	for _, f := range fields {
		if f.flag == "" {
			rest++
			variadic = variadic || f.variadic
		}
	}
	if rest < 0 {
		project := addScope(fs)
		pos, err := parse(fs, args)
		if err != nil {
			return scope{}, nil, err
		}
		if len(pos) > 0 {
			return scope{}, nil, usagef("unexpected argument %q", pos[0])
		}
		sc, err := resolveScope(e, *project)
		return sc, nil, err
	}
	maxRest := rest
	if variadic {
		maxRest = -1
	}
	sc, ref, pos, err := itemCommand(e, fs, args, rest, maxRest)
	return sc, append([]string{string(ref)}, pos...), err
}

// setField sets field v from the command line's values for it.
func setField(v reflect.Value, vals []string) error {
	switch v.Kind() {
	case reflect.String:
		v.SetString(vals[0])
	case reflect.Slice:
		for _, s := range vals {
			v.Set(reflect.Append(v, reflect.ValueOf(s).Convert(v.Type().Elem())))
		}
	case reflect.Map:
		m, err := tagFlags(vals)
		if err != nil {
			return err
		}
		for k, s := range m {
			if v.IsNil() {
				v.Set(reflect.MakeMap(v.Type()))
			}
			v.SetMapIndex(reflect.ValueOf(k).Convert(v.Type().Key()), reflect.ValueOf(s).Convert(v.Type().Elem()))
		}
	}
	return nil
}
