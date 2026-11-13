package store_test

import (
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 35 (code-central-543) added a Project's Auto-accept kinds and a
// Move's auto-accept cause. The real chain migrates a format-34 store
// copying every file unchanged; a Project from before reads every kind
// off, and its Moves read with no cause.
func TestTheRealChainMigratesAFormat34StoreToTheAutoAcceptFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 34\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/4.yaml"),
		"title: Work\nstate: Done\nspec: 2\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\n"+
			"moves:\n  - from: In Review\n    to: Done\n    actor: human\n    time: 2026-01-02T03:04:05Z\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 34 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 34", m)
	}
	after := snapshot(t, dir)
	for rel, content := range before {
		if rel != "store.yaml" && after[rel] != content {
			t.Errorf("%s changed:\n%s", rel, after[rel])
		}
	}
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		for _, k := range store.AutoAcceptKinds {
			if p.AutoAccepts(k) {
				t.Errorf("auto-accept %s is on after migrating, want every kind off", k)
			}
		}
		if tk, _ := r.Ticket("acme", 4); len(tk.Moves) != 1 || tk.Moves[0].Cause != "" {
			t.Errorf("moves after migrating = %+v, want one with no cause", tk.Moves)
		}
		return nil
	})

	// Kinds turned on and a cause recorded after migrating read back.
	if err := s.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.SetAutoAccept(store.AutoAcceptMerge, true)
		p.SetAutoAccept(store.AutoAcceptGrilling, true)
		return w.PutProject(p)
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", 4)
		tk.Moves[0].Cause = store.CauseProject
		return w.PutTicket("acme", tk)
	}); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, s)
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if !p.AutoAccepts(store.AutoAcceptGrilling) || !p.AutoAccepts(store.AutoAcceptMerge) || p.AutoAccepts(store.AutoAcceptSpec) {
			t.Errorf("auto-accept = %v, want grilling and merge on", p.AutoAccept)
		}
		if tk, _ := r.Ticket("acme", 4); tk.Moves[0].Cause != store.CauseProject {
			t.Errorf("cause = %q, want project", tk.Moves[0].Cause)
		}
		return nil
	})
}
