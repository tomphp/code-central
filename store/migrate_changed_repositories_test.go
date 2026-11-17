package store_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/store"
)

// Format 44 (code-central-854) added a Ticket's Changed Repositories and
// the Repository Name of its pushed commits, Spec merge and pull request.
// The real chain migrates a format-43 store copying every file unchanged:
// a Ticket from before keeps its branch and commit and has no list, and
// its records have no Repository Name, so all read as the Home
// Repository's.
func TestTheRealChainMigratesAFormat43StoreToTheChangedRepositoriesFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 43\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	writeFile(t, filepath.Join(dir, "projects/acme/specs/1.md"),
		"---\nid: 1\ntitle: S\nlabels: []\ntags: {}\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\nedits: []\n---\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"),
		"title: T\nstate: Accepted\nspec: 1\nlabels: []\ntags: {}\nblocked_by: []\nreplacements: []\n"+
			"created: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\nbranch: acme-2\ncommit: abc123\n"+
			"pushed_commits:\n  - commit: abc123\n    pushed: 2026-01-02T03:04:05Z\n"+
			"pull_request:\n  number: 7\n  link: https://example.com/7\n  state: open\n"+
			"claims: []\nrounds: []\nfailure_reports: []\ncomments: []\nmoves: []\nedits: []\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 43 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 43", m)
	}
	after := snapshot(t, dir)
	for rel, content := range before {
		if rel != "store.yaml" && after[rel] != content {
			t.Errorf("%s changed:\n%s", rel, after[rel])
		}
	}
	s.View(func(r store.Reader) error {
		tk, _ := r.Ticket("acme", 2)
		if tk.Branch != "acme-2" || tk.Commit != "abc123" || tk.Repositories != nil {
			t.Errorf("ticket after migrating: branch %q commit %q repositories %+v, want its branch and commit and no list", tk.Branch, tk.Commit, tk.Repositories)
		}
		if tk.PushedCommits[0].Repository != "" || tk.PullRequests[0].Repository != "" {
			t.Errorf("pushed commit %+v and pull request %+v, want no Repository Name", tk.PushedCommits[0], tk.PullRequests)
		}
		return nil
	})

	// Changed Repositories and Repository Names written after migrating
	// read back as written.
	at := time.Date(2026, 1, 3, 4, 5, 6, 0, time.UTC)
	changed := []store.ChangedRepository{
		{Name: "acme", Branch: "acme-2", Commit: "abc123", Landed: &store.Landed{Commit: "def456", Time: at}},
		{Name: "api", Branch: "acme-2", Commit: "fed789"},
	}
	if err := s.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", 2)
		tk.Repositories = changed
		tk.PushedCommits = append(tk.PushedCommits, store.PushedCommit{Repository: "api", Commit: "fed789", Pushed: at})
		tk.PullRequests[0].Repository = "api"
		tk.SpecMerge = &store.SpecMerge{Repository: "api", Commit: "aaa111", Merged: at}
		return w.PutTicket("acme", tk)
	}); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, s)
	s.View(func(r store.Reader) error {
		tk, _ := r.Ticket("acme", 2)
		if !reflect.DeepEqual(tk.Repositories, changed) {
			t.Errorf("repositories %+v, want %+v", tk.Repositories, changed)
		}
		if tk.PushedCommits[1].Repository != "api" || tk.PullRequests[0].Repository != "api" || tk.SpecMerge.Repository != "api" {
			t.Errorf("pushed commit %+v, pull request %+v, Spec merge %+v, want each in api", tk.PushedCommits[1], tk.PullRequests, tk.SpecMerge)
		}
		return nil
	})
	if got := snapshot(t, dir)["projects/acme/tickets/2.yaml"]; !strings.Contains(got, "repositories:\n  - name: acme\n") {
		t.Errorf("ticket file:\n%s", got)
	}
}

// A Ticket file naming a Changed Repository twice, or by a name that
// breaks the Repository Name rules, doesn't load.
func TestATicketsChangedRepositoriesAreCheckedOnLoad(t *testing.T) {
	for name, list := range map[string]string{
		"twice":    "repositories:\n  - name: api\n    branch: b\n    commit: c\n  - name: api\n    branch: b\n    commit: d\n",
		"bad name": "repositories:\n  - name: Api!\n    branch: b\n    commit: c\n",
		"no name":  "repositories:\n  - branch: b\n    commit: c\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "code-central")
			writeFile(t, filepath.Join(dir, "store.yaml"), "format: 44\n")
			writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
				"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
			writeFile(t, filepath.Join(dir, "projects/acme/specs/1.md"),
				"---\nid: 1\ntitle: S\nlabels: []\ntags: {}\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\nedits: []\n---\n")
			writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"),
				"title: T\nstate: Done\nspec: 1\nlabels: []\ntags: {}\nblocked_by: []\nreplacements: []\n"+
					"created: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\n"+list+
					"claims: []\nrounds: []\nfailure_reports: []\ncomments: []\nmoves: []\nedits: []\n")
			if _, err := store.Open(dir); err == nil || !strings.Contains(err.Error(), "repositories[") {
				t.Fatalf("open: %v, want a load error naming the repositories field", err)
			}
		})
	}
}
