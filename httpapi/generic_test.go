package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/ops"
)

// The fixtures: small input types, each exercising one decoding rule.

type itemInput struct {
	Ticket api.Ref `json:"ticket" path:"n"`
	Body   string  `json:"body"`
}

type kind string

type queryInput struct {
	Ticket    api.Ref  `json:"ticket" path:"n"`
	State     []kind   `json:"state,omitempty"`
	Title     string   `json:"title,omitempty"`
	Labels    []string `json:"labels,omitempty" query:"label"`
	Claimable *bool    `json:"claimable,omitempty"`
	Full      bool     `json:"full,omitempty"`
	Parts     []string `json:"-" query:"part"`
	Hidden    bool     `json:"-"`
}

type tagsInput struct {
	Title string            `json:"title,omitempty"`
	Tags  map[string]string `json:"tags,omitempty" query:"tag"`
}

// DecodeQuery reads tag=<key>=<value> parameters.
func (in *tagsInput) DecodeQuery(q url.Values) error {
	for _, p := range q["tag"] {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return &api.Error{Code: api.Invalid, Field: "tag", Message: "a Tag filter is tag=<key>=<value>"}
		}
		if in.Tags == nil {
			in.Tags = map[string]string{}
		}
		in.Tags[k] = v
	}
	return nil
}

// echo is a definition over In, for echoed to serve.
func echo[In any](http string) *ops.Definition[In, map[string]any] {
	return &ops.Definition[In, map[string]any]{Name: "echo", HTTP: http}
}

// echoed serves a call with its key and the input the door decoded.
func echoed(key string, in any) (any, error) {
	return map[string]any{"key": key, "in": in}, nil
}

// serving is an ops.Service that serves every call with serve, as an
// operation's binding would.
type serving struct {
	ops.Service
	serve func(key string, in any) (any, error)
}

func (s serving) Dispatch(_ context.Context, call ops.Call) (any, error) {
	return s.serve(call.Key, call.In)
}

// serveOp mounts op's generic handler on a mux, as Register does, served
// by serve.
func serveOp(t *testing.T, op ops.Operation, serve func(key string, in any) (any, error)) http.Handler {
	t.Helper()
	h, err := handle(op)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	method, pattern := op.Route()
	mux.HandleFunc(method+" "+pattern, func(w http.ResponseWriter, r *http.Request) {
		v, err := h(serving{serve: serve}, r)
		if err != nil {
			WriteError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	})
	return mux
}

func call(h http.Handler, method, path, body string) (int, map[string]any) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestThePathBindsTheItemAndWinsOverTheBody(t *testing.T) {
	h := serveOp(t, echo[itemInput]("POST /api/p/{key}/tickets/{n}/comment"), echoed)
	status, out := call(h, "POST", "/api/p/acme/tickets/7/comment", `{"ticket":"acme-99","body":"Hi"}`)
	if status != 200 || out["key"] != "acme" {
		t.Fatalf("%d %v", status, out)
	}
	in := out["in"].(map[string]any)
	if in["ticket"] != "7" || in["body"] != "Hi" {
		t.Fatalf("input %v: the path's 7 should win over the body's acme-99", in)
	}
}

func TestABodyIsDecodedStrictly(t *testing.T) {
	h := serveOp(t, echo[itemInput]("POST /api/p/{key}/tickets/{n}/comment"), echoed)
	status, out := call(h, "POST", "/api/p/acme/tickets/7/comment", `{"colour":"red"}`)
	if status != 422 || out["code"] != string(api.Invalid) || out["field"] != "colour" {
		t.Fatalf("%d %v", status, out)
	}
}

func TestAQueryIsDecodedByItsTags(t *testing.T) {
	h := serveOp(t, echo[queryInput]("GET /api/p/{key}/tickets/{n}"), echoed)
	status, out := call(h, "GET", "/api/p/acme/tickets/7?state=Ready&state=Draft&title=A&label=x&label=y&claimable=false&full=true&part=claims", "")
	if status != 200 {
		t.Fatalf("%d %v", status, out)
	}
	// The echo's JSON came back through a map, so its keys are sorted.
	got, _ := json.Marshal(out["in"])
	want := `{"claimable":false,"full":true,"labels":["x","y"],"state":["Ready","Draft"],"ticket":"7","title":"A"}`
	if string(got) != want {
		t.Fatalf("input %s, want %s", got, want)
	}
	// Parts is json:"-" with a query tag: on the query, not in the JSON.
	status, out = call(h, "GET", "/api/p/acme/tickets/7", "")
	got, _ = json.Marshal(out["in"])
	if status != 200 || string(got) != `{"ticket":"7"}` {
		t.Fatalf("%d %s: an absent parameter leaves its field zero", status, got)
	}
}

func TestAHiddenFieldWithAQueryTagIsDecodedFromTheQuery(t *testing.T) {
	type partsInput struct {
		Parts  []string `json:"-" query:"part"`
		Hidden bool     `json:"-"`
	}
	op := &ops.Definition[partsInput, []string]{Name: "echo", HTTP: "GET /api/p/{key}/things"}
	h := serveOp(t, op, func(_ string, in any) (any, error) {
		if in.(*partsInput).Hidden {
			t.Fatal("a json:\"-\" field with no query tag was set")
		}
		return in.(*partsInput).Parts, nil
	})
	req := httptest.NewRequest("GET", "/api/p/acme/things?part=claims&part=edits", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `["claims","edits"]` {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	status, out := call(h, "GET", "/api/p/acme/things?Hidden=true", "")
	if status != 422 || out["message"] != `unknown query parameter "Hidden" (allowed: part)` {
		t.Fatalf("%d %v", status, out)
	}
}

func TestAnUnknownQueryParameterIsRejectedNamingTheAllowedOnes(t *testing.T) {
	h := serveOp(t, echo[queryInput]("GET /api/p/{key}/tickets/{n}"), echoed)
	status, out := call(h, "GET", "/api/p/acme/tickets/7?ticket=7", "")
	if status != 422 || out["field"] != "ticket" {
		t.Fatalf("%d %v: the path-bound field is not a parameter", status, out)
	}
	want := `unknown query parameter "ticket" (allowed: state, title, label, claimable, full, part)`
	if out["message"] != want {
		t.Fatalf("message %q, want %q", out["message"], want)
	}
	status, out = call(h, "GET", "/api/p/acme/tickets/7?full=maybe", "")
	if status != 422 || out["field"] != "full" || out["message"] != `full is true or false, not "maybe"` {
		t.Fatalf("%d %v", status, out)
	}
}

func TestAnInputTypeDecodesItsOwnOddQueryShape(t *testing.T) {
	h := serveOp(t, echo[tagsInput]("GET /api/p/{key}/things"), echoed)
	status, out := call(h, "GET", "/api/p/acme/things?title=T&tag=effort=high&tag=model=fable", "")
	if status != 200 {
		t.Fatalf("%d %v", status, out)
	}
	got, _ := json.Marshal(out["in"])
	if want := `{"tags":{"effort":"high","model":"fable"},"title":"T"}`; string(got) != want {
		t.Fatalf("input %s, want %s", got, want)
	}
	status, out = call(h, "GET", "/api/p/acme/things?tag=effort", "")
	if status != 422 || out["field"] != "tag" {
		t.Fatalf("%d %v: the type's own error should come through", status, out)
	}
	status, out = call(h, "GET", "/api/p/acme/things?colour=red", "")
	if status != 422 || out["message"] != `unknown query parameter "colour" (allowed: title, tag)` {
		t.Fatalf("%d %v: the custom parameter is still named as allowed", status, out)
	}
}

func TestADefinitionTheHandlerCannotDecodeFailsNamingTheField(t *testing.T) {
	type bad struct {
		Tags map[string]string `json:"tags" query:"tag"`
	}
	type pathless struct {
		Ticket api.Ref `json:"ticket"`
	}
	type doubled struct {
		Ticket api.Ref `json:"ticket" path:"n"`
		Spec   api.Ref `json:"spec" path:"n"`
	}
	for _, c := range []struct {
		name string
		op   ops.Operation
		want string
	}{
		{"an undecodable query field", echo[bad]("GET /api/p/{key}/things"), "field tags is a map[string]string, which no query tag decodes"},
		{"a pattern with {n} and no path tag", echo[pathless]("GET /api/p/{key}/things/{n}"), "disagree on {n}"},
		{"two path tags", echo[doubled]("POST /api/p/{key}/things/{n}/do"), "fields ticket and spec both bind {n}"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := handle(c.op)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %v, want %q", err, c.want)
			}
		})
	}
}

func TestRoutesDeriveFromTheDefinitions(t *testing.T) {
	routes := Routes()
	if routes["comment"] != "POST /api/p/{key}/tickets/{n}/comment" {
		t.Fatalf("comment: %q", routes["comment"])
	}
	if routes["claim"] != "POST /api/p/{key}/claim" {
		t.Fatalf("claim: %q", routes["claim"])
	}
}
