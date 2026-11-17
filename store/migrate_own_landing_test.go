package store_test

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 50 (code-central-899) let a Repository, the Home included, hold
// an Own Landing and a Push remote. The real chain migrates a format-49
// store copying every file unchanged: each Repository then follows the
// Default Landing with the Project's Push remote, and an Own Landing and
// a Push remote read back as written.
func TestTheRealChainMigratesAFormat49StoreToTheOwnLandingFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 49\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n"+
			"landing:\n  method: merge\n  push: true\n  remote: origin\nrepositories:\n"+
			"  - name: acme\n    home: true\n  - name: web\n    clone_url: git@github.com:acme/web.git\n    base: main\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 49 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 49", m)
	}
	after := snapshot(t, dir)
	for rel, content := range before {
		if rel != "store.yaml" && after[rel] != content {
			t.Errorf("%s changed:\n%s", rel, after[rel])
		}
	}
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		for _, rp := range p.RepositoryList() {
			if rp.OwnLanding != (store.OwnLanding{}) || rp.PushRemote != "" {
				t.Errorf("%s = %+v, want it following the Default Landing with the Project's Push remote", rp.Name, rp)
			}
		}
		return nil
	})

	want := []store.Repository{
		{Name: "acme", Home: true, OwnLanding: store.OwnLanding{Method: store.LandingMerge}},
		{Name: "web", CloneURL: "git@github.com:acme/web.git", Base: "main", OwnLanding: store.OwnLanding{Method: store.LandingPullRequest}, PushRemote: "upstream"},
		{Name: "notes", Base: "main", LocalPath: "/src/notes", OwnLanding: store.OwnLanding{Method: store.LandingBranch, Push: true}},
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

// The Home alone with an Own Landing or a Push remote is stored, so it
// isn't lost to "a list holding only the Home Repository is stored as
// none"; and loading normalises Pull Request's Push, as for the Default
// Landing.
func TestTheHomeAloneKeepsItsOwnLandingAndPullRequestsPushIsNormalised(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), fmt.Sprintf("format: %d\n", store.Format))
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nrepositories:\n"+
			"  - name: acme\n    home: true\n    landing:\n      method: pull_request\n      push: true\n    push_remote: upstream\n")
	s := openFile(t, dir)
	want := []store.Repository{{Name: "acme", Home: true, OwnLanding: store.OwnLanding{Method: store.LandingPullRequest}, PushRemote: "upstream"}}
	check := func(s *store.FileStore) {
		t.Helper()
		s.View(func(r store.Reader) error {
			p, _ := r.Project("acme")
			if got := p.RepositoryList(); !reflect.DeepEqual(got, want) {
				t.Errorf("repositories = %+v, want %+v", got, want)
			}
			return nil
		})
	}
	check(s)
	if err := s.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.SetRepositories(p.RepositoryList())
		p.DisplayName = "Acme 2"
		return w.PutProject(p)
	}); err != nil {
		t.Fatal(err)
	}
	check(reopen(t, s))
}
