package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/ops"
	"github.com/tomoram/code-central/store"
)

// The fixtures: small types, one or two per derivation rule.

type color string

type shapes struct {
	Ticket  core.Ref          `json:"ticket" desc:"The Ticket." required:"true"`
	Title   string            `json:"title" desc:"A title." required:"true"`
	Note    *string           `json:"note,omitempty" desc:"A note."`
	Colour  color             `json:"colour,omitempty" desc:"Red or blue." enum:"red,blue"`
	Done    bool              `json:"done,omitempty" desc:"Done or not."`
	Open    *bool             `json:"open,omitempty" desc:"Open (true) or not (false)."`
	Labels  []string          `json:"labels,omitempty" desc:"Some Labels."`
	Colours []color           `json:"colours,omitempty" desc:"Some colours."`
	Refs    []core.Ref        `json:"refs,omitempty" desc:"Some IDs."`
	Tags    map[string]string `json:"tags,omitempty" desc:"Some Tags."`
	Rows    []row             `json:"rows,omitempty" desc:"Some rows."`
	Local   []string          `json:"-"`
	Hidden  string            `json:"secret" mcp:"-" desc:"Not for MCP."`
}

type row struct {
	Name   string `json:"name" desc:"A name." required:"true"`
	Branch string `json:"branch" desc:"A branch."`
}

type base struct {
	Ticket core.Ref `json:"ticket" desc:"The Ticket." required:"true"`
}

type embedded struct {
	base
	Full bool `json:"full,omitempty" desc:"The whole thing."`
}

func TestSchemaDerivesEachShape(t *testing.T) {
	got, err := schema(reflect.TypeFor[shapes](), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"ticket", "title"},
		"properties": map[string]any{
			"ticket":  map[string]any{"type": []any{"string", "integer"}, "description": "The Ticket. An ID: <key>-<n> or <n>."},
			"title":   map[string]any{"type": "string", "description": "A title."},
			"note":    map[string]any{"type": "string", "description": "A note."},
			"colour":  map[string]any{"type": "string", "enum": []any{"red", "blue"}, "description": "Red or blue."},
			"done":    map[string]any{"type": "boolean", "description": "Done or not."},
			"open":    map[string]any{"type": "boolean", "description": "Open (true) or not (false)."},
			"labels":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Some Labels."},
			"colours": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Some colours."},
			"refs":    map[string]any{"type": "array", "items": map[string]any{"type": []any{"string", "integer"}}, "description": "Some IDs."},
			"tags":    map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Some Tags."},
			"rows": map[string]any{"type": "array", "description": "Some rows.", "items": map[string]any{
				"type": "object", "additionalProperties": false, "required": []any{"name"},
				"properties": map[string]any{
					"name":   map[string]any{"type": "string", "description": "A name."},
					"branch": map[string]any{"type": "string", "description": "A branch."},
				},
			}},
		},
	}
	if g, w := asJSON(t, got), asJSON(t, want); g != w {
		t.Fatalf("schema\n%s\nwant\n%s", g, w)
	}
}

func TestSchemaFlattensEmbeddedStructsAndTakesOverrides(t *testing.T) {
	got, err := schema(reflect.TypeFor[embedded](), func(name string) (string, bool) {
		if name == "full" {
			return "Return the whole Ticket.", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"type": "object", "additionalProperties": false, "required": []any{"ticket"},
		"properties": map[string]any{
			"ticket": map[string]any{"type": []any{"string", "integer"}, "description": "The Ticket. An ID: <key>-<n> or <n>."},
			"full":   map[string]any{"type": "boolean", "description": "Return the whole Ticket."},
		},
	}
	if g, w := asJSON(t, got), asJSON(t, want); g != w {
		t.Fatalf("schema\n%s\nwant\n%s", g, w)
	}
}

func TestSchemaWithNothingRequiredHasNoRequiredList(t *testing.T) {
	got, err := schema(reflect.TypeFor[struct {
		Title string `json:"title,omitempty" desc:"A title."`
	}](), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["required"]; ok {
		t.Fatalf("schema %v lists required", got)
	}
}

func TestSchemaFailsNamingTheFieldItCannotExpress(t *testing.T) {
	for _, c := range []struct {
		name string
		typ  reflect.Type
		want string
	}{
		{"a number", reflect.TypeFor[struct {
			Count int `json:"count" desc:"A count."`
		}](), "field count: a int has no schema"},
		{"a list of numbers", reflect.TypeFor[struct {
			Counts []int `json:"counts" desc:"Counts."`
		}](), "field counts: a list of int has no schema"},
		{"a map of lists", reflect.TypeFor[struct {
			Groups map[string][]string `json:"groups" desc:"Groups."`
		}](), "field groups: a map[string][]string has no schema"},
		{"an enum on a bool", reflect.TypeFor[struct {
			Done bool `json:"done" desc:"Done." enum:"yes,no"`
		}](), "field done: enum on a bool, not a string"},
		{"no description", reflect.TypeFor[struct {
			Title string `json:"title"`
		}](), "field title has no desc tag or description override"},
		{"no description in a nested object", reflect.TypeFor[struct {
			Rows []struct {
				Name string `json:"name"`
			} `json:"rows" desc:"Rows."`
		}](), "field rows: field name has no desc tag or description override"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := schema(c.typ, nil)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %v, want %q", err, c.want)
			}
		})
	}
}

// asJSON is v as indented JSON, through a round trip so that []string and
// []any compare equal.
func asJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var round any
	if err := json.Unmarshal(b, &round); err != nil {
		t.Fatal(err)
	}
	out, err := json.MarshalIndent(round, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// A derived tool on an operation whose path has no {key} checks the
// Project itself, as get_workflow's hand-written tool does, so that every
// tool on an unknown key answers unknown_project.
func TestADerivedToolOnAPathWithoutAKeyChecksTheProject(t *testing.T) {
	c := core.New(store.NewMemory(), clock.NewFake(time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)), &ids.Sequence{})
	ctx := context.Background()
	if _, err := c.CreateProject(ctx, core.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	called := 0
	pathless := &ops.Definition[struct{}, string]{
		Name: "ping", HTTP: "GET /api/ping", Doc: "Ping.",
		Call: func(*core.Core, context.Context, string, struct{}) (string, error) { called++; return "pong", nil },
	}
	keyed := &ops.Definition[struct{}, string]{
		Name: "ping", HTTP: "GET /api/p/{key}/ping", Doc: "Ping.",
		Call: func(*core.Core, context.Context, string, struct{}) (string, error) { called++; return "pong", nil },
	}
	for _, c2 := range []struct {
		name string
		def  ops.Operation
		key  string
		want core.Code
	}{
		{"no {key}, unknown Project", pathless, "nowhere", core.UnknownProject},
		{"no {key}, known Project", pathless, "acme", ""},
		{"{key}, unknown Project: the Core answers", keyed, "nowhere", ""},
	} {
		t.Run(c2.name, func(t *testing.T) {
			tl, err := derive(c2.def)
			if err != nil {
				t.Fatal(err)
			}
			_, err = tl.call(ctx, c, c2.key, []byte(`{}`))
			var e *core.Error
			switch {
			case c2.want == "" && err != nil:
				t.Fatalf("error %v", err)
			case c2.want != "" && (!errors.As(err, &e) || e.Code != c2.want):
				t.Fatalf("error %v, want %s", err, c2.want)
			}
		})
	}
	if called != 2 {
		t.Fatalf("the call ran %d times, want 2: never on an unknown Project without {key}", called)
	}
}
