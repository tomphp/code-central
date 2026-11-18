package store_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Format 53 (code-central-1022) added an auto fix Ticket's Auto-fix mark.
// The real chain migrates a format-52 store copying every file
// unchanged, and its Tickets then read as unmarked; a set mark reads
// back as written.
func TestTheRealChainMigratesAFormat52StoreToTheAutoFixMarkFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 52\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"),
		"title: Fix it\nstate: Ready\nspec: 1\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 52 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 52", m)
	}
	after := snapshot(t, dir)
	for rel, content := range before {
		if rel != "store.yaml" && after[rel] != content {
			t.Errorf("%s changed:\n%s", rel, after[rel])
		}
	}
	s.View(func(r store.Reader) error {
		if tk, _ := r.Ticket("acme", 2); tk.AutoFix != nil {
			t.Errorf("auto fix mark = %+v, want none", tk.AutoFix)
		}
		return nil
	})

	mark := &domain.AutoFixMark{Attempt: 2, Repository: "api", Commit: "abc123", Covers: []int{5, 8}, Previous: 3}
	if err := s.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", 2)
		tk.AutoFix = mark
		return w.PutTicket("acme", tk)
	}); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, s)
	s.View(func(r store.Reader) error {
		if tk, _ := r.Ticket("acme", 2); !reflect.DeepEqual(tk.AutoFix, mark) {
			t.Errorf("auto fix mark = %+v, want %+v", tk.AutoFix, mark)
		}
		return nil
	})
}
