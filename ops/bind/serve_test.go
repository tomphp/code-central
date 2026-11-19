package bind

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/ops"
	"github.com/tomoram/code-central/store"
)

// serveWith is c served with the real bindings and these besides.
func serveWith(c *core.Core, extra ...binding) ops.Service {
	m := make(map[ops.Operation]binding, len(bindings)+len(extra))
	for op, b := range bindings {
		m[op] = b
	}
	for _, b := range extra {
		m[b.op] = b
	}
	return served{Core: c, bindings: m}
}

// A key given to an operation whose path has no {key} must name a
// Project, so that every MCP tool on an unknown key answers
// unknown_project: get_workflow has no check of its own.
func TestDispatchChecksAKeyGivenToAnOperationWithoutOne(t *testing.T) {
	c := core.New(store.NewMemory(), clock.NewFake(time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)), &ids.Sequence{})
	ctx := context.Background()
	if _, err := c.CreateProject(ctx, api.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	called := 0
	ping := func(*core.Core, context.Context, string, struct{}) (string, error) { called++; return "pong", nil }
	pathless := &ops.Definition[struct{}, string]{Name: "ping", HTTP: "GET /api/ping", Doc: "Ping."}
	keyed := &ops.Definition[struct{}, string]{Name: "ping", HTTP: "GET /api/p/{key}/ping", Doc: "Ping."}
	s := serveWith(c, bind(pathless, ping), bind(keyed, ping))
	for _, c2 := range []struct {
		name string
		def  ops.Operation
		key  string
		want api.Code
	}{
		{"no {key}, unknown Project", pathless, "nowhere", api.UnknownProject},
		{"get_workflow, unknown Project", ops.GetWorkflow, "nowhere", api.UnknownProject},
		{"no {key}, known Project", pathless, "acme", ""},
		{"no {key}, no key given", pathless, "", ""},
		{"{key}, unknown Project: the Core answers", keyed, "nowhere", ""},
	} {
		t.Run(c2.name, func(t *testing.T) {
			_, err := s.Dispatch(ctx, ops.Call{Op: c2.def, Key: c2.key, In: &struct{}{}})
			var e *api.Error
			switch {
			case c2.want == "" && err != nil:
				t.Fatalf("error %v", err)
			case c2.want != "" && (!errors.As(err, &e) || e.Code != c2.want):
				t.Fatalf("error %v, want %s", err, c2.want)
			}
		})
	}
	if called != 3 {
		t.Fatalf("the call ran %d times, want 3: never on an unknown Project without {key}", called)
	}
}

type echoInput struct {
	Title string `json:"title"`
}

// A call with no decoded input has its Args decoded strictly into one, as
// MCP's arguments are: an unknown field is invalid.
func TestDispatchDecodesArgsStrictlyWhenTheDoorDecodedNoInput(t *testing.T) {
	echo := &ops.Definition[echoInput, string]{Name: "echo", HTTP: "POST /api/p/{key}/echo", Doc: "Echo."}
	s := serveWith(nil, bind(echo, func(_ *core.Core, _ context.Context, _ string, in echoInput) (string, error) { return in.Title, nil }))
	if got, err := s.Dispatch(context.Background(), ops.Call{Op: echo, Key: "acme", Args: []byte(`{"title":"Search"}`)}); err != nil || got != "Search" {
		t.Fatalf("decoded %v, err %v", got, err)
	}
	_, err := s.Dispatch(context.Background(), ops.Call{Op: echo, Key: "acme", Args: []byte(`{"colour":"red"}`)})
	var e *api.Error
	if !errors.As(err, &e) || e.Code != api.Invalid {
		t.Fatalf("unknown field: err %v, want invalid", err)
	}
}

// A definition with no binding is an error at dispatch, which the
// cross-door suite's binding check keeps from happening.
func TestDispatchOfAnUnboundDefinitionIsAnError(t *testing.T) {
	unbound := &ops.Definition[struct{}, string]{Name: "unbound", HTTP: "GET /api/p/{key}/unbound", Doc: "Unbound."}
	if _, err := Serve(nil).Dispatch(context.Background(), ops.Call{Op: unbound, Key: "acme", In: &struct{}{}}); err == nil {
		t.Fatal("dispatched an unbound definition")
	}
}
