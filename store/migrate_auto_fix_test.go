package store_test

import (
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Format 52 (code-central-1022) added a Project's Auto-fix setting. The
// real chain migrates a format-51 store copying every file unchanged, and
// the Project then reads Auto-fix as never set; a set entry reads back as
// written.
func TestTheRealChainMigratesAFormat51StoreToTheAutoFixFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 51\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nci:\n  tracking: true\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 51 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 51", m)
	}
	after := snapshot(t, dir)
	for rel, content := range before {
		if rel != "store.yaml" && after[rel] != content {
			t.Errorf("%s changed:\n%s", rel, after[rel])
		}
	}
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if p.CI.AutoFix != (domain.AutoFix{}) {
			t.Errorf("auto fix = %+v, want never set", p.CI.AutoFix)
		}
		return nil
	})

	set := domain.AutoFix{On: true, Instructions: "Mine.", Limit: 7}
	if err := s.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.CI.AutoFix = set
		return w.PutProject(p)
	}); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, s)
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if p.CI.AutoFix != set {
			t.Errorf("auto fix = %+v, want %+v", p.CI.AutoFix, set)
		}
		return nil
	})
}
