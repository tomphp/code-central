package core_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
)

// resultJSON is a write's result as a client sees it.
func resultJSON(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// wantAcknowledgement checks a write's result is the Acknowledgement of
// id, changing exactly changed, with edit number edit (0: none).
func wantAcknowledgement(t *testing.T, v any, id string, updated time.Time, changed []string, edit int) {
	t.Helper()
	out := resultJSON(t, v)
	got := []string{}
	for _, c := range out["changed"].([]any) {
		got = append(got, c.(string))
	}
	if out["id"] != id || out["updated"] != updated.Format(time.RFC3339) || !slices.Equal(got, changed) {
		t.Fatalf("acknowledgement %v, want id %s, updated %s, changed %v", out, id, updated, changed)
	}
	if e, ok := out["edit"]; edit == 0 && ok || edit != 0 && e != float64(edit) {
		t.Fatalf("acknowledgement %v, want edit %d", out, edit)
	}
	for _, k := range []string{"title", "body", "edits"} {
		if _, ok := out[k]; ok {
			t.Fatalf("acknowledgement %v has the record's %s", out, k)
		}
	}
}

func TestEditSpecAcknowledgesTheFieldsItChanged(t *testing.T) {
	f := defining(t)
	f.clock.Advance(time.Minute)
	v, err := f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "1", Body: ptr("New.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-1", t0.Add(time.Minute), []string{"body", "edits"}, 1)

	v, err = f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "1", Title: ptr("Sign in"), Body: ptr("Newer.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-1", t0.Add(time.Minute), []string{"title", "body", "edits"}, 2)

	f.clock.Advance(time.Minute)
	v, err = f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "1", Body: ptr("Newer.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-1", t0.Add(time.Minute), []string{}, 0)
}

func TestCreateSpecAcknowledgesWithTheNewID(t *testing.T) {
	f := defining(t)
	v, err := f.core.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Search", Body: "Find things.\n"})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-3", t0, []string{"title", "body"}, 0)
	v, err = f.core.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Empty"})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-4", t0, []string{"title"}, 0)
}

func TestLinkedSpecWritesAcknowledge(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	v, err := f.core.CreateLinkedSpec(integration, "acme", core.CreateLinkedSpecInput{Title: "Login", Body: "Users log in.\n", Link: ghLink("10")})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-1", t0, []string{"title", "body", "link"}, 0)

	f.clock.Advance(time.Minute)
	v, err = f.core.UpdateLinkedSpec(integration, "acme", core.UpdateLinkedSpecInput{Spec: "1", Body: ptr("New.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-1", t0.Add(time.Minute), []string{"body", "edits"}, 1)
	v, err = f.core.UpdateLinkedSpec(integration, "acme", core.UpdateLinkedSpecInput{Spec: "1", Body: ptr("New.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-1", t0.Add(time.Minute), []string{}, 0)
}

func TestSpecWritesWithFullReturnTheSpec(t *testing.T) {
	f := defining(t)
	f.clock.Advance(time.Minute)
	writes := map[string]func() (any, error){
		"create_spec": func() (any, error) {
			return f.core.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Search", Full: true})
		},
		"edit_spec": func() (any, error) {
			return f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "1", Body: ptr("New.\n"), Full: true})
		},
		"create_linked_spec": func() (any, error) {
			return f.core.CreateLinkedSpec(integration, "acme", core.CreateLinkedSpecInput{Title: "Epic", Link: ghLink("10"), Full: true})
		},
		"update_linked_spec": func() (any, error) {
			return f.core.UpdateLinkedSpec(integration, "acme", core.UpdateLinkedSpecInput{Spec: "4", Title: ptr("Epic 2"), Full: true})
		},
	}
	for _, name := range []string{"create_spec", "edit_spec", "create_linked_spec", "update_linked_spec"} {
		v, err := writes[name]()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out := resultJSON(t, v)
		get, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: core.Ref(out["id"].(string))})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// The full read starts with the Spec's fields; it leaves out an
		// empty edits, which the Spec gives as [].
		want := resultJSON(t, get)
		for k := range want {
			if _, ok := out[k]; !ok {
				delete(want, k)
			}
		}
		if _, ok := want["edits"]; !ok {
			want["edits"] = []any{}
		}
		if !reflect.DeepEqual(out, want) {
			t.Fatalf("%s with full: %v, want the Spec %v", name, out, want)
		}
	}
}
