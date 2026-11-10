package core_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// The lighter get_spec (ticket 134): parts, Sections and the outline.

const specBody = "Why.\n\n## Solution\n\nDo it.\n\n### Detail\n\nSmall print.\n\n## Testing Decisions\n\nSeams.\n"

// sectionedSpec is Spec acme-1 with specBody, one edit and two Tickets.
func (f fixture) sectionedSpec(t *testing.T) {
	t.Helper()
	f.work(t, store.Ready, store.Draft)
	body := specBody
	if _, err := f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "1", Body: &body}); err != nil {
		t.Fatalf("edit_spec: %v", err)
	}
}

// readSpec is get_spec's result as a caller sees its JSON.
func (f fixture) readSpec(t *testing.T, in core.GetSpecInput) map[string]any {
	t.Helper()
	v, err := f.core.GetSpec(ctx, "acme", in)
	if err != nil {
		t.Fatalf("get_spec %+v: %v", in, err)
	}
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

func keys(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// always is every key a lighter read returns whatever is asked for (Link
// and Requirement are left out here because acme-1 is neither).
var always = []string{"created", "finished", "id", "labels", "outline", "state_summary", "tags", "title", "updated"}

var specOutline = []any{
	map[string]any{"text": "Solution", "level": float64(2)},
	map[string]any{"text": "Detail", "level": float64(3)},
	map[string]any{"text": "Testing Decisions", "level": float64(2)},
}

func TestGetSpecWithNeitherInputIsTheFullSpec(t *testing.T) {
	f := newFixture(t)
	f.sectionedSpec(t)
	full := f.readSpec(t, core.GetSpecInput{Spec: "acme-1"})
	want := []string{"body", "created", "edits", "finished", "id", "labels", "state_summary", "tags", "tickets", "title", "updated"}
	if got := keys(full); !slices.Equal(got, want) {
		t.Fatalf("keys %v, want %v: no outline on a full read", got, want)
	}
	if full["body"] != specBody || len(full["edits"].([]any)) != 1 || len(full["tickets"].([]any)) != 2 {
		t.Fatalf("full Spec %v", full)
	}
	// An empty body and no edits are still there, not left out.
	empty := f.spec(t, "acme", "Empty")
	none := ""
	if _, err := f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: core.Ref(empty.ID), Body: &none}); err != nil {
		t.Fatal(err)
	}
	v, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: core.Ref(empty.ID)})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	if s := string(b); !strings.Contains(s, `"body":""`) || !strings.Contains(s, `"tickets":[]`) {
		t.Fatalf("an empty Spec's full read %s", s)
	}
}

func TestGetSpecReturnsOnlyTheAskedForParts(t *testing.T) {
	f := newFixture(t)
	f.sectionedSpec(t)
	for _, c := range []struct {
		parts []string
		extra []string
	}{
		{[]string{"body"}, []string{"body"}},
		{[]string{"tickets"}, []string{"tickets"}},
		{[]string{"edits"}, []string{"edits"}},
		{[]string{"edits", "body"}, []string{"body", "edits"}},
		{[]string{"body", "tickets", "edits"}, []string{"body", "edits", "tickets"}},
	} {
		t.Run(strings.Join(c.parts, ","), func(t *testing.T) {
			out := f.readSpec(t, core.GetSpecInput{Spec: "acme-1", Parts: c.parts})
			want := append(slices.Clone(always), c.extra...)
			slices.Sort(want)
			if got := keys(out); !slices.Equal(got, want) {
				t.Fatalf("keys %v, want %v", got, want)
			}
			// The outline covers the whole body, even when the body isn't returned.
			if !reflect.DeepEqual(out["outline"], specOutline) {
				t.Fatalf("outline %v", out["outline"])
			}
			if out["id"] != "acme-1" || out["title"] != "Login" || out["finished"] != false {
				t.Fatalf("always-returned fields %v", out)
			}
			if summary := out["state_summary"].(map[string]any); summary["Ready"] != float64(1) || summary["Draft"] != float64(1) {
				t.Fatalf("state_summary %v", summary)
			}
		})
	}
}

func TestGetSpecWithSectionsCutsTheBody(t *testing.T) {
	f := newFixture(t)
	f.sectionedSpec(t)
	out := f.readSpec(t, core.GetSpecInput{Spec: "acme-1", Sections: []string{"testing decisions", "Solution"}})
	if want := "## Solution\n\nDo it.\n\n### Detail\n\nSmall print.\n\n## Testing Decisions\n\nSeams."; out["body"] != want {
		t.Fatalf("body %q, want %q", out["body"], want)
	}
	// Without parts, only the body comes back, with the outline (ticket 318).
	want := append(slices.Clone(always), "body")
	slices.Sort(want)
	if got := keys(out); !slices.Equal(got, want) || !reflect.DeepEqual(out["outline"], specOutline) {
		t.Fatalf("sections without parts: keys %v, want %v; outline %v", got, want, out["outline"])
	}
	// An explicit parts still wins.
	out = f.readSpec(t, core.GetSpecInput{Spec: "acme-1", Parts: []string{"body", "tickets"}, Sections: []string{"Solution"}})
	if len(out["tickets"].([]any)) != 2 || out["edits"] != nil || out["body"] != "## Solution\n\nDo it.\n\n### Detail\n\nSmall print." {
		t.Fatalf("sections with parts body and tickets %v", out)
	}
	out = f.readSpec(t, core.GetSpecInput{Spec: "acme-1", Parts: []string{"body"}, Sections: []string{"Detail"}})
	if out["body"] != "### Detail\n\nSmall print." || out["tickets"] != nil || out["edits"] != nil {
		t.Fatalf("parts and sections %v", out)
	}
}

func TestGetSpecRejectsAContradictoryOrMistypedRead(t *testing.T) {
	f := newFixture(t)
	f.sectionedSpec(t)
	_, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-1", Parts: []string{"body", "history"}})
	wantCode(t, err, core.Invalid, "parts")
	_, err = f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-1", Parts: []string{"tickets"}, Sections: []string{"Solution"}})
	wantCode(t, err, core.Invalid, "sections")
	_, err = f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-1", Sections: []string{"Solution", "Solutoin"}})
	wantCode(t, err, core.Invalid, "sections")
	msg := err.(*core.Error).Message
	for _, want := range []string{`"Solutoin"`, "Solution", "Detail", "Testing Decisions"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q doesn't name %s", msg, want)
		}
	}
}

func TestALighterReadKeepsTheLinkAndTheRequirement(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	if out := f.readSpec(t, core.GetSpecInput{Spec: "acme-1", Parts: []string{"tickets"}}); out["link"] == nil {
		t.Fatalf("a linked Spec's lighter read has no link: %v", out)
	}
	f = newFixture(t)
	f.requirement(t)
	if out := f.readSpec(t, core.GetSpecInput{Spec: "acme-2", Parts: []string{"edits"}}); out["requirement"] == nil {
		t.Fatalf("a Requirement's lighter read has no requirement: %v", out)
	}
}
