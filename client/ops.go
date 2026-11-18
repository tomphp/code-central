package client

// The generic call: one request for every operation defined in ops. It
// fills {key} and {n} in the definition's path, encodes a GET's input as
// its query by the same tags the HTTP door decodes it with, and sends a
// POST's input as the body, so the client can't build a different path
// from the server's.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fieldwalk"
	"github.com/tomoram/code-central/ops"
)

// queryEncoder is an input type with a query shape the struct tags can't
// express, such as the finders' tag=<key>=<value>: the counterpart of the
// HTTP door's QueryDecoder.
type queryEncoder interface {
	EncodeQuery(q url.Values)
}

func init() {
	// A GET input the client can't encode is a programming error; fail at
	// start-up, naming the operation and the field, as the doors do.
	for _, op := range ops.Definitions() {
		if method, _ := op.Route(); method != http.MethodGet {
			continue
		}
		custom := op.Input().Implements(reflect.TypeFor[queryEncoder]())
		for _, f := range fieldwalk.AllFields(op.Input()) {
			if _, ok := ops.QueryParam(f); ok && !ops.QueryScalar(f.Type) && !custom {
				panic(fmt.Sprintf("client: %s: field %s is a %s, which no query tag encodes; give the input type an EncodeQuery method", op.Op().Name, f.Name, f.Type))
			}
		}
	}
}

// request sends def's request for in under the Project key and decodes
// the answer into out.
func request[In, Out any](ctx context.Context, c *Client, def *ops.Definition[In, Out], key string, in In, out any) error {
	method, pattern := def.Route()
	path := pathOf(pattern, key, in)
	if method == http.MethodGet {
		if enc := queryOf(in).Encode(); enc != "" {
			path += "?" + enc
		}
		return c.do(ctx, method, path, nil, out)
	}
	return c.do(ctx, method, path, in, out)
}

// call is request with the answer decoded as the definition's result.
func call[In, Out any](ctx context.Context, c *Client, def *ops.Definition[In, Out], key string, in In) (Out, error) {
	var out Out
	return out, request(ctx, c, def, key, in, &out)
}

// written is call for an acknowledging write: the record when full was
// asked for, else the Acknowledgement.
func written[In, V any](ctx context.Context, c *Client, def *ops.Definition[In, core.Written[V]], key string, in In, full bool) (core.Written[V], error) {
	var raw json.RawMessage
	if err := request(ctx, c, def, key, in, &raw); err != nil {
		return core.Written[V]{}, err
	}
	return decodeWritten[V](raw, full)
}

// pathOf is pattern with {key} filled from key and the placeholder a
// field's path tag names, {n} or {stage}, from that field, both escaped.
func pathOf(pattern, key string, in any) string {
	v := reflect.ValueOf(in)
	for _, f := range fieldwalk.Fields(v.Type()) {
		if name := f.Tag.Get("path"); name != "" {
			pattern = strings.ReplaceAll(pattern, "{"+name+"}", url.PathEscape(v.FieldByIndex(f.Index).String()))
		}
	}
	return strings.ReplaceAll(pattern, "{key}", url.PathEscape(key))
}

// queryOf encodes in as a GET's query: each field under ops.QueryParam,
// leaving zero values out; a string once, a list repeated, a bool as
// true, a *bool as true or false. The type encodes what no tag can
// itself.
func queryOf(in any) url.Values {
	q := url.Values{}
	v := reflect.ValueOf(in)
	for _, f := range fieldwalk.AllFields(v.Type()) {
		name, ok := ops.QueryParam(f)
		if !ok {
			continue
		}
		fv := v.FieldByIndex(f.Index)
		switch {
		case fv.Kind() == reflect.String:
			if fv.String() != "" {
				q.Set(name, fv.String())
			}
		case fv.Kind() == reflect.Bool:
			if fv.Bool() {
				q.Set(name, "true")
			}
		case fv.Kind() == reflect.Pointer && fv.Type().Elem().Kind() == reflect.Bool:
			if !fv.IsNil() {
				q.Set(name, strconv.FormatBool(fv.Elem().Bool()))
			}
		case fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() == reflect.String:
			for i := 0; i < fv.Len(); i++ {
				q.Add(name, fv.Index(i).String())
			}
		}
	}
	if e, ok := in.(queryEncoder); ok {
		e.EncodeQuery(q)
	}
	return q
}
