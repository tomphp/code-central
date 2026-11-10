package store_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 17 (code-central-289) added the keys of the comments written for
// a Link. The real chain migrates a format-16 store, whose Links have
// none, copying every file unchanged; a Link from before reads as having
// nothing written, so every comment due for it is still written once.
func TestTheRealChainMigratesAFormat16StoreToTheLinkWrittenKeysFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 16\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\ngithub_issues:\n  on: true\n  repository: acme/web\n  spec: 1\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"),
		"title: Login is broken\nstate: Draft\nspec: 1\nlink:\n  integration: github\n  id: acme/web#7\n  external_state: open\nawaiting_triage: true\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-16.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 16, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 16 with its backup", m)
	}
	after := snapshot(t, dir)
	for rel, content := range before {
		if rel != "store.yaml" && after[rel] != content {
			t.Errorf("%s changed:\n%s", rel, after[rel])
		}
	}
	if got := after["store.yaml"]; got != fmt.Sprintf("format: %d\n", store.Format) {
		t.Errorf("store.yaml = %q", got)
	}
	s.View(func(r store.Reader) error {
		tk, _ := r.Ticket("acme", 2)
		if len(tk.Link.Written) != 0 || tk.Link.ID != "acme/web#7" || !tk.AwaitingTriage {
			t.Errorf("ticket after migrating = %+v, want its Link and awaiting triage kept, with nothing written", tk)
		}
		return nil
	})

	// Keys written after migrating read back.
	if err := s.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", 2)
		tk.Link.Written = []string{"wont-fix"}
		return w.PutTicket("acme", tk)
	}); err != nil {
		t.Fatal(err)
	}
	s.View(func(r store.Reader) error {
		if tk, _ := r.Ticket("acme", 2); !slices.Equal(tk.Link.Written, []string{"wont-fix"}) {
			t.Errorf("written = %q, want wont-fix", tk.Link.Written)
		}
		return nil
	})
}
