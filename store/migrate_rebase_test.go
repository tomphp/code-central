package store_test

import (
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 54 (code-central-1072) added a Project's Rebase landing option.
// The real chain migrates a format-53 store copying every file unchanged,
// and its Projects then read with Rebase off; Rebase on reads back as
// written.
func TestTheRealChainMigratesAFormat53StoreToTheRebaseFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 53\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nlanding:\n    method: pull_request\n    push: false\n    remote: origin\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 53 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 53", m)
	}
	after := snapshot(t, dir)
	for rel, content := range before {
		if rel != "store.yaml" && after[rel] != content {
			t.Errorf("%s changed:\n%s", rel, after[rel])
		}
	}
	s.View(func(r store.Reader) error {
		if p, _ := r.Project("acme"); p.Landing.Rebase {
			t.Error("rebase on after migrating a store that never stored it")
		}
		return nil
	})

	if err := s.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.Landing.Rebase = true
		return w.PutProject(p)
	}); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, s)
	s.View(func(r store.Reader) error {
		if p, _ := r.Project("acme"); !p.Landing.Rebase {
			t.Error("rebase off after writing it on")
		}
		return nil
	})
}
