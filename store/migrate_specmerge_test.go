package store_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/tomoram/code-central/store"
)

// Format 22 added a Ticket's Spec merge. The real chain migrates a
// format-21 store: a Ticket file without it reads as not merged, and one
// with it keeps its commit and time.
func TestTheRealChainMigratesAFormat21StoreToTheSpecMergeFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 21\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"), "key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	head := "state: Accepted\nspec: 1\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\n"
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"), "title: Plain\n"+head)
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/3.yaml"), "title: Merged\n"+head+"spec_merge:\n  commit: abc123\n  merged: 2026-01-03T03:04:05Z\n")

	s := openFile(t, dir)
	s.View(func(r store.Reader) error {
		if tk, _ := r.Ticket("acme", 2); tk.SpecMerge != nil {
			t.Errorf("spec merge of a plain Ticket = %+v, want none", tk.SpecMerge)
		}
		tk, _ := r.Ticket("acme", 3)
		want := store.SpecMerge{Commit: "abc123", Merged: time.Date(2026, 1, 3, 3, 4, 5, 0, time.UTC)}
		if tk.SpecMerge == nil || *tk.SpecMerge != want {
			t.Errorf("spec merge = %+v, want %+v", tk.SpecMerge, want)
		}
		return nil
	})
}
