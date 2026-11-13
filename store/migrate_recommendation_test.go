package store_test

import (
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 34 (code-central-541) added a spec Stage's Spec recommendation.
// The real chain migrates a format-33 store, whose Tickets have none,
// copying every file unchanged; a spec Stage finished before reads as
// having no recommendation.
func TestTheRealChainMigratesAFormat33StoreToTheRecommendationFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 33\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/4.yaml"),
		"title: Spec\nstate: In Review\nspec: 2\nlabels:\n  - stage:spec\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 33 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 33", m)
	}
	after := snapshot(t, dir)
	for rel, content := range before {
		if rel != "store.yaml" && after[rel] != content {
			t.Errorf("%s changed:\n%s", rel, after[rel])
		}
	}
	s.View(func(r store.Reader) error {
		if tk, _ := r.Ticket("acme", 4); tk.State != store.InReview || tk.Recommendation != "" {
			t.Errorf("ticket after migrating = %+v, want In Review with no recommendation", tk)
		}
		return nil
	})

	// A recommendation recorded after migrating reads back.
	if err := s.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", 4)
		tk.Recommendation = store.RecommendSplit
		return w.PutTicket("acme", tk)
	}); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, s)
	s.View(func(r store.Reader) error {
		if tk, _ := r.Ticket("acme", 4); tk.Recommendation != store.RecommendSplit {
			t.Errorf("recommendation = %q, want split", tk.Recommendation)
		}
		return nil
	})
}
