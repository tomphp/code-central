package store_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 48 (code-central-884) let a Repository other than the Home leave
// out its Clone URL when it has a local path: it is local-only. The real
// chain migrates a format-47 store copying every file unchanged, and a
// local-only Repository then reads back with no Clone URL.
func TestTheRealChainMigratesAFormat47StoreToTheLocalOnlyRepositoriesFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 47\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nrepositories:\n"+
			"  - name: acme\n    home: true\n  - name: web\n    clone_url: git@github.com:acme/web.git\n    base: main\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 47 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 47", m)
	}
	after := snapshot(t, dir)
	for rel, content := range before {
		if rel != "store.yaml" && after[rel] != content {
			t.Errorf("%s changed:\n%s", rel, after[rel])
		}
	}

	want := []store.Repository{
		{Name: "acme", Home: true},
		{Name: "web", CloneURL: "git@github.com:acme/web.git", Base: "main"},
		{Name: "notes", Base: "main", LocalPath: "/src/notes"},
	}
	if err := s.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.SetRepositories(want)
		return w.PutProject(p)
	}); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, s)
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if got := p.RepositoryList(); !reflect.DeepEqual(got, want) {
			t.Errorf("repositories = %+v, want %+v", got, want)
		}
		return nil
	})
}
