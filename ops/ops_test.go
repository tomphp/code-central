package ops_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fieldwalk"
	"github.com/tomoram/code-central/ops"
)

func TestEveryOperationIsListedOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, op := range ops.All() {
		if seen[op.Name] {
			t.Errorf("operation %s is listed twice", op.Name)
		}
		seen[op.Name] = true
		if got, ok := ops.Lookup(op.Name); !ok || got != op {
			t.Errorf("Lookup(%s) = %+v, %v", op.Name, got, ok)
		}
	}
	if _, ok := ops.Lookup("fly"); ok {
		t.Error("Lookup found an operation that doesn't exist")
	}
}

func TestEveryDefinitionIsListedWithItsOp(t *testing.T) {
	for _, d := range ops.Definitions() {
		op := d.Op()
		if listed, ok := ops.Lookup(op.Name); !ok || listed != op {
			t.Errorf("definition %s: listed as %+v, %v", op.Name, listed, ok)
		}
		if got, ok := ops.DefinitionOf(op.Name); !ok || got != d {
			t.Errorf("DefinitionOf(%s) is not the definition", op.Name)
		}
		method, pattern := d.Route()
		if (method != "GET" && method != "POST") || !strings.HasPrefix(pattern, "/api/") {
			t.Errorf("%s: route %q %q", op.Name, method, pattern)
		}
		if (method == "GET") == op.Write {
			t.Errorf("%s: a write is a POST and a read a GET, not %s", op.Name, method)
		}
		if d.Description() == "" || strings.ContainsAny(d.Description(), "\r\n") {
			t.Errorf("%s: description %q is not one line", op.Name, d.Description())
		}
		if d.Input().Kind() != reflect.Struct {
			t.Errorf("%s: input %s is not a struct", op.Name, d.Input())
		}
		if reflect.TypeOf(d.NewInput()) != reflect.PointerTo(d.Input()) {
			t.Errorf("%s: NewInput is a %T", op.Name, d.NewInput())
		}
		var bound []string
		for _, f := range fieldwalk.Fields(d.Input()) {
			if p := f.Tag.Get("path"); p != "" {
				bound = append(bound, "{"+p+"}")
			}
		}
		if strings.Contains(pattern, "{n}") != (len(bound) == 1 && bound[0] == "{n}") {
			t.Errorf("%s: pattern %s binds %v", op.Name, pattern, bound)
		}
	}
	if _, ok := ops.DefinitionOf("get_workflow"); !ok {
		t.Error("get_workflow has a definition")
	}
}

func TestQueryParamFollowsTheTags(t *testing.T) {
	type in struct {
		Ticket core.Ref          `json:"ticket" path:"n"`
		Title  string            `json:"title,omitempty"`
		Labels []string          `json:"labels,omitempty" query:"label"`
		Parts  []string          `json:"-" query:"part"`
		Hidden bool              `json:"-"`
		Tags   map[string]string `json:"tags,omitempty"`
	}
	var got []string
	var scalars []string
	for _, f := range fieldwalk.AllFields(reflect.TypeFor[in]()) {
		if name, ok := ops.QueryParam(f); ok {
			got = append(got, name)
		}
		if ops.QueryScalar(f.Type) {
			scalars = append(scalars, f.Name)
		}
	}
	if want := []string{"title", "label", "part", "tags"}; !slices.Equal(got, want) {
		t.Fatalf("parameters %v, want %v", got, want)
	}
	if want := []string{"ticket", "title", "labels", "Parts", "Hidden"}; !slices.Equal(scalars, want) {
		t.Fatalf("scalars %v, want %v: a map needs the type's own query methods", scalars, want)
	}
}
