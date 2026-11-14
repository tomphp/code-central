package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 38 (code-central-659) added the Chat row to a Project's Models.
// The real chain migrates a format-37 store copying every file unchanged;
// a Project from before reads an empty Chat row, and one set after
// migrating reads back.
func TestTheRealChainMigratesAFormat37StoreToTheChatRowFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 37\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nmodels:\n  default:\n    model: opus\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-37.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 37, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 37 with its backup", m)
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
		p, _ := r.Project("acme")
		if p.Models.Chat != (store.ModelRow{}) || p.Models.Default.Model != "opus" {
			t.Errorf("models = %+v, want the default kept and an empty Chat row", p.Models)
		}
		return nil
	})

	if err := s.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.Models.Chat = store.ModelRow{Model: "sonnet", Effort: "low"}
		return w.PutProject(p)
	}); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, s)
	s.View(func(r store.Reader) error {
		if p, _ := r.Project("acme"); p.Models.Chat != (store.ModelRow{Model: "sonnet", Effort: "low"}) {
			t.Errorf("chat row = %+v, want sonnet at low", p.Models.Chat)
		}
		return nil
	})
}
