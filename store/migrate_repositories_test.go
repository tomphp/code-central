package store_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 42 (code-central-851) added a Project's Repositories. The real
// chain migrates a format-41 store copying every file unchanged, and a
// Project from before has the Home Repository alone, named by its key.
func TestTheRealChainMigratesAFormat41StoreToTheRepositoriesFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 41\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
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
	home := []store.Repository{{Name: "acme", Home: true}}
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if got := p.RepositoryList(); !reflect.DeepEqual(got, home) {
			t.Errorf("repositories after migrating = %+v, want the Home Repository alone", got)
		}
		return nil
	})

	// Repositories added after migrating read back in their order, and
	// the Home Repository alone is stored as none.
	added := []store.Repository{
		{Name: "web", CloneURL: "git@github.com:acme/web.git", Base: "main", LocalPath: "/src/web"},
		{Name: "acme", Home: true},
		{Name: "api", CloneURL: "https://github.com/acme/api", Base: "develop"},
	}
	put := func(rs []store.Repository) {
		t.Helper()
		if err := s.Update(func(w store.Writer) error {
			p, _ := w.Project("acme")
			p.SetRepositories(rs)
			return w.PutProject(p)
		}); err != nil {
			t.Fatal(err)
		}
		s = reopen(t, s)
	}
	put(added)
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if got := p.RepositoryList(); !reflect.DeepEqual(got, added) {
			t.Errorf("repositories = %+v, want %+v", got, added)
		}
		return nil
	})
	put(home)
	if got := snapshot(t, dir)["projects/acme/project.yaml"]; strings.Contains(got, "repositories") {
		t.Errorf("project.yaml with the Home Repository alone holds repositories:\n%s", got)
	}
}

func TestLoadRefusesAProjectsRepositoriesThatBreakTheirRules(t *testing.T) {
	// A Project Key may be longer than a Repository Name, and still names
	// its Home Repository.
	long := strings.Repeat("a", store.MaxRepositoryName+1)
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 42\n")
	writeFile(t, filepath.Join(dir, "projects/"+long+"/project.yaml"),
		"key: "+long+"\ndisplay_name: Long\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nrepositories:\n"+
			"  - name: "+long+"\n    home: true\n  - name: web\n    clone_url: u\n    base: main\n")
	if s, err := store.Open(dir); err != nil {
		t.Fatalf("open with a long Project Key: %v", err)
	} else {
		s.Close()
	}

	for name, tc := range map[string]struct{ repos, field string }{
		"a bad name":            {"  - name: acme\n    home: true\n  - name: Web\n    clone_url: u\n    base: main\n", "repositories[1].name"},
		"a name twice":          {"  - name: acme\n    home: true\n  - name: web\n    clone_url: u\n    base: main\n  - name: web\n    clone_url: v\n    base: main\n", "repositories[2].name"},
		"no Home":               {"  - name: web\n    clone_url: u\n    base: main\n", "repositories"},
		"Home not the key":      {"  - name: other\n    home: true\n", "repositories[0].name"},
		"Home with a base":      {"  - name: acme\n    home: true\n    base: main\n", "repositories[0]"},
		"no Clone URL":          {"  - name: acme\n    home: true\n  - name: web\n    base: main\n", "repositories[1].clone_url"},
		"no Base":               {"  - name: acme\n    home: true\n  - name: web\n    clone_url: u\n", "repositories[1].base"},
		"two Home Repositories": {"  - name: acme\n    home: true\n  - name: acme\n    home: true\n", "repositories[1].name"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "code-central")
			writeFile(t, filepath.Join(dir, "store.yaml"), "format: 42\n")
			writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
				"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nrepositories:\n"+tc.repos)
			_, err := store.Open(dir)
			le, ok := err.(*store.LoadError)
			if !ok || le.Field != tc.field {
				t.Fatalf("open = %v, want a LoadError on %s", err, tc.field)
			}
		})
	}
}

func TestRepositoryNamesKeepTheirRules(t *testing.T) {
	for name, want := range map[string]bool{
		"web": true, "web-app": true, "api_2": true, "acme.io": true, "9lives": true,
		"": false, "Web": false, "-web": false, ".web": false, "_web": false, "web app": false, "web/app": false,
		strings.Repeat("a", store.MaxRepositoryName): true, strings.Repeat("a", store.MaxRepositoryName+1): false,
	} {
		if got := store.ValidRepositoryName(name); got != want {
			t.Errorf("ValidRepositoryName(%q) = %v, want %v", name, got, want)
		}
	}
}
