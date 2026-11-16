package store_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 42 (code-central-852) added the repository_missing Focus Kind.
// The real chain migrates a format-41 store copying every file unchanged,
// and a Project can then turn the new kind off.
func TestTheRealChainMigratesAFormat41StoreToTheRepositoryMissingFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 41\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nfocus_kinds_off:\n  - base_missing\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 41 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 41", m)
	}
	after := snapshot(t, dir)
	for rel, content := range before {
		if rel != "store.yaml" && after[rel] != content {
			t.Errorf("%s changed:\n%s", rel, after[rel])
		}
	}

	if err := s.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.FocusKindsOff = []string{"base_missing", "repository_missing"}
		return w.PutProject(p)
	}); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(t, dir)["projects/acme/project.yaml"]; !strings.Contains(got, "  - repository_missing\n") {
		t.Errorf("project.yaml after turning repository_missing off:\n%s", got)
	}
}
