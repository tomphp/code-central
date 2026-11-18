package httpapi

// The generic handler: one handler serves every operation defined in ops
// (ops.Definitions). It decodes the path, query or body into the
// operation's input type, applies the definition's HTTP defaults, calls
// the operation and returns its result or error for Register to write.

import (
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fieldwalk"
	"github.com/tomoram/code-central/ops"
)

// QueryDecoder is an input type with a query shape the struct tags can't
// express, such as the finders' tag=<key>=<value>. The generic handler
// decodes every field it can from its query or json tag, rejects unknown
// parameters, then calls DecodeQuery for the rest.
type QueryDecoder interface {
	DecodeQuery(q url.Values) error
}

// BodyErrorDescriber is an input type that adds to the error of a body
// that fails to decode, such as naming the fields it accepts.
type BodyErrorDescriber interface {
	DescribeDecodeError(err error) error
}

// derived are the handlers derived from the definitions, by operation
// name. A definition whose input the handler can't decode fails here, at
// start-up, naming the operation and the field.
var derived = map[string]handlerFunc{}

func init() {
	for _, op := range ops.Definitions() {
		h, err := handle(op)
		if err != nil {
			panic(fmt.Sprintf("httpapi: %s: %v", op.Op().Name, err))
		}
		derived[op.Op().Name] = h
	}
}

// Routes maps every operation to its HTTP method and path pattern
// (net/http ServeMux syntax): the definitions', and during the migration
// the legacy map's. The cross-door suite checks it against ops.All().
func Routes() map[string]string {
	out := maps.Clone(legacyRoutes)
	for _, op := range ops.Definitions() {
		method, pattern := op.Route()
		out[op.Op().Name] = method + " " + pattern
	}
	return out
}

// LegacyRoutes names the operations the hand-written handlers still
// serve, for the migration guard.
func LegacyRoutes() []string {
	var out []string
	for name := range legacyRoutes {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// handle is op's handler. A POST decodes the body strictly; a GET decodes
// the query. Then {n} is bound to the path-tagged field, winning over the
// body, {key} is the Project key, and the HTTP defaults are applied.
func handle(op ops.Operation) (handlerFunc, error) {
	method, pattern := op.Route()
	path, err := pathField(op.Input(), pattern)
	if err != nil {
		return nil, err
	}
	var q *queryShape
	if method == http.MethodGet {
		if q, err = queryShapeOf(op.Input()); err != nil {
			return nil, err
		}
	}
	return func(c *core.Core, r *http.Request) (any, error) {
		in := op.NewInput()
		if q != nil {
			if err := q.decode(r.URL.Query(), in); err != nil {
				return nil, err
			}
		} else if err := decodeBody(r, in); err != nil {
			if d, ok := in.(BodyErrorDescriber); ok {
				err = d.DescribeDecodeError(err)
			}
			return nil, err
		}
		if path != nil {
			reflect.ValueOf(in).Elem().FieldByIndex(path.Index).SetString(r.PathValue("n"))
		}
		op.ApplyHTTPDefaults(in)
		return op.Invoke(r.Context(), c, r.PathValue("key"), in)
	}, nil
}

// pathField is the field of t that pattern's {n} binds to, tagged
// path:"n", or nil when the pattern has no {n}.
func pathField(t reflect.Type, pattern string) (*fieldwalk.Field, error) {
	var found *fieldwalk.Field
	for _, f := range fieldwalk.Fields(t) {
		tag := f.Tag.Get("path")
		switch {
		case tag == "":
		case tag != "n":
			return nil, fmt.Errorf("field %s binds {%s}, but a path binds only {n}", f.Name, tag)
		case found != nil:
			return nil, fmt.Errorf("fields %s and %s both bind {n}", found.Name, f.Name)
		case f.Type.Kind() != reflect.String:
			return nil, fmt.Errorf("field %s binds {n} but is a %s, not a string", f.Name, f.Type)
		default:
			found = &f
		}
	}
	if (found != nil) != strings.Contains(pattern, "{n}") {
		return nil, fmt.Errorf("pattern %s and the path tags disagree on {n}", pattern)
	}
	return found, nil
}

// queryShape is how a GET's input type decodes from its query: each
// parameter's name and field, and whether the type decodes the rest.
type queryShape struct {
	params []queryParam
	// allowed are every parameter's name, in field order, for the unknown
	// parameter error.
	allowed []string
	// custom is set when the type implements QueryDecoder, which then
	// reads the fields no tag can.
	custom bool
}

type queryParam struct {
	name  string
	field fieldwalk.Field
}

// queryShapeOf reads t's query shape: each field's parameter is
// ops.QueryParam. A field no tag can decode needs the type to implement
// QueryDecoder.
func queryShapeOf(t reflect.Type) (*queryShape, error) {
	s := &queryShape{custom: reflect.PointerTo(t).Implements(reflect.TypeFor[QueryDecoder]())}
	for _, f := range fieldwalk.AllFields(t) {
		name, ok := ops.QueryParam(f)
		if !ok {
			continue
		}
		s.allowed = append(s.allowed, name)
		if !ops.QueryScalar(f.Type) {
			if !s.custom {
				return nil, fmt.Errorf("field %s is a %s, which no query tag decodes; give the input type a DecodeQuery method", f.Name, f.Type)
			}
			continue
		}
		s.params = append(s.params, queryParam{name, f})
	}
	return s, nil
}

// decode fills in, a pointer to the input type, from q. Repeated
// parameters fill lists, true and false fill booleans, and an unknown
// parameter is invalid, naming the allowed ones.
func (s *queryShape) decode(q url.Values, in any) error {
	if err := onlyParams(q, s.allowed...); err != nil {
		return err
	}
	v := reflect.ValueOf(in).Elem()
	for _, p := range s.params {
		if !q.Has(p.name) {
			continue
		}
		f := v.FieldByIndex(p.field.Index)
		switch f.Kind() {
		case reflect.String:
			f.SetString(q.Get(p.name))
		case reflect.Bool, reflect.Pointer:
			b, err := boolParam(q, p.name)
			if err != nil {
				return err
			}
			if f.Kind() == reflect.Bool {
				f.SetBool(*b)
			} else {
				f.Set(reflect.ValueOf(b))
			}
		case reflect.Slice:
			list := reflect.MakeSlice(f.Type(), 0, len(q[p.name]))
			for _, s := range q[p.name] {
				list = reflect.Append(list, reflect.ValueOf(s).Convert(f.Type().Elem()))
			}
			f.Set(list)
		}
	}
	if s.custom {
		return in.(QueryDecoder).DecodeQuery(q)
	}
	return nil
}

// boolParam reads an optional true/false query parameter.
func boolParam(q url.Values, name string) (*bool, error) {
	if !q.Has(name) {
		return nil, nil
	}
	b, err := strconv.ParseBool(q.Get(name))
	if err != nil {
		return nil, &core.Error{Code: core.Invalid, Field: name, Message: fmt.Sprintf("%s is true or false, not %q", name, q.Get(name))}
	}
	return &b, nil
}
