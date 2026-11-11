package store_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 25 (code-central-414) labels each CI Rule's Spec and each
// Project's Issues Spec long-lived, once.
func TestTheRealChainLabelsCIRuleAndIssuesSpecsLongLivedOnce(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 24\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		`key: acme
display_name: Acme
created: 2026-01-02T03:04:05Z
mode: committed
ci:
  tracking: true
  provider: github_actions
  repository: acme/web
  rules:
    - spec: 2
      workflow: ci.yml
      branch: main
      state: Ready
      created: 2026-01-02T03:04:05Z
github_issues:
  on: true
  repository: acme/web
  spec: 3
`)
	spec := func(n, title string) {
		writeFile(t, filepath.Join(dir, "projects/acme/specs/"+n+".md"),
			"---\nid: "+n+"\ntitle: \""+title+"\"\nlabels:\n  - ui\ntags: {}\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\nedits: []\n---\nBody\n")
	}
	spec("1", "Login")
	spec("2", "CI: ci.yml on main")
	spec("3", "GitHub Issues: acme/web")

	labels := func(s *store.FileStore, n int) []string {
		var got []string
		s.View(func(r store.Reader) error {
			sp, ok := r.Spec("acme", n)
			if !ok {
				t.Fatalf("spec %d missing", n)
			}
			got = sp.Labels
			return nil
		})
		return got
	}
	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 24 {
		t.Fatalf("migrated = %+v", m)
	}
	for n, want := range map[int][]string{1: {"ui"}, 2: {"ui", "long-lived"}, 3: {"ui", "long-lived"}} {
		if got := labels(s, n); !slices.Equal(got, want) {
			t.Errorf("spec %d labels = %v, want %v", n, got, want)
		}
	}
	if err := s.Update(func(w store.Writer) error {
		sp, _ := w.Spec("acme", 2)
		sp.Labels = []string{"ui"}
		return w.PutSpec("acme", sp)
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s = openFile(t, dir)
	if s.Migrated() != nil {
		t.Fatalf("reopening migrated again: %+v", s.Migrated())
	}
	if got := labels(s, 2); !slices.Equal(got, []string{"ui"}) {
		t.Errorf("the removed Label came back: %v", got)
	}
}
