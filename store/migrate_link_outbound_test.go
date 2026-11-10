package store_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 18 (code-central-290) added a Link's outbound flag. The real
// chain migrates a format-17 store copying every file unchanged; a Link
// from before reads as owned, not outbound, with its written keys kept.
func TestTheRealChainMigratesAFormat17StoreToTheLinkOutboundFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 17\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\ngithub_issues:\n  on: true\n  repository: acme/web\n  spec: 1\n")
	writeFile(t, filepath.Join(dir, "projects/acme/specs/2.md"),
		"---\nid: 2\ntitle: Login is broken\nlabels: []\ntags: {}\nlink:\n  integration: github\n  id: acme/web#7\n  external_state: open\n  written:\n    - wont-fix\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\nedits: []\n---\n# Login is broken\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-17.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 17, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 17 with its backup", m)
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
		sp, _ := r.Spec("acme", 2)
		if sp.Link.Outbound || !sp.Link.Owned() || !slices.Equal(sp.Link.Written, []string{"wont-fix"}) {
			t.Errorf("spec after migrating = %+v, want its Link owned, not outbound, with its written keys", sp.Link)
		}
		return nil
	})

	// An outbound Link written after migrating reads back, and doesn't
	// own the definition.
	if err := s.Update(func(w store.Writer) error {
		sp, _ := w.Spec("acme", 2)
		sp.Link.Outbound = true
		return w.PutSpec("acme", sp)
	}); err != nil {
		t.Fatal(err)
	}
	s.View(func(r store.Reader) error {
		if sp, _ := r.Spec("acme", 2); !sp.Link.Outbound || sp.Link.Owned() || !sp.Link.Attached() {
			t.Errorf("link = %+v, want outbound, attached and not owned", sp.Link)
		}
		return nil
	})
}
