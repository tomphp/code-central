package store_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 45 (code-central-856) added a Ticket's pull requests in its
// other Repositories and the mark to close one. The real chain migrates a
// format-44 store copying every file unchanged: a Ticket's one pull
// request stays the Home Repository's.
func TestTheRealChainMigratesAFormat44StoreToThePullRequestsFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 44\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	writeFile(t, filepath.Join(dir, "projects/acme/specs/1.md"),
		"---\nid: 1\ntitle: S\nlabels: []\ntags: {}\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\nedits: []\n---\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"),
		"title: T\nstate: Accepted\nspec: 1\nlabels: []\ntags: {}\nblocked_by: []\nreplacements: []\n"+
			"created: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\nbranch: acme-2\ncommit: abc123\n"+
			"pull_request:\n  number: 7\n  link: https://example.com/7\n  state: open\n"+
			"claims: []\nrounds: []\nfailure_reports: []\ncomments: []\nmoves: []\nedits: []\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 44 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 44", m)
	}
	after := snapshot(t, dir)
	for rel, content := range before {
		if rel != "store.yaml" && after[rel] != content {
			t.Errorf("%s changed:\n%s", rel, after[rel])
		}
	}
	home := store.PullRequest{Number: 7, Link: "https://example.com/7", State: store.PROpen}
	s.View(func(r store.Reader) error {
		if tk, _ := r.Ticket("acme", 2); !reflect.DeepEqual(tk.PullRequests, []store.PullRequest{home}) {
			t.Errorf("pull requests after migrating = %+v, want the Home Repository's alone", tk.PullRequests)
		}
		return nil
	})

	// A pull request in another Repository, and one marked to close, read
	// back as written; the Home Repository's stays pull_request.
	home.Close = true
	want := []store.PullRequest{home, {Repository: "api", Number: 3, Link: "https://example.com/api/3", State: store.PRMerged, MergeCommit: "def456"}}
	if err := s.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", 2)
		tk.PullRequests = want
		return w.PutTicket("acme", tk)
	}); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, s)
	s.View(func(r store.Reader) error {
		if tk, _ := r.Ticket("acme", 2); !reflect.DeepEqual(tk.PullRequests, want) {
			t.Errorf("pull requests = %+v, want %+v", tk.PullRequests, want)
		}
		return nil
	})
	got := snapshot(t, dir)["projects/acme/tickets/2.yaml"]
	if !strings.Contains(got, "pull_request:\n  number: 7\n") || !strings.Contains(got, "pull_requests:\n  - repository: api\n") || !strings.Contains(got, "close: true") {
		t.Errorf("ticket file:\n%s", got)
	}
}

// A Ticket file with two pull requests in one Repository doesn't load.
func TestATicketsPullRequestsAreOnePerRepository(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 45\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	writeFile(t, filepath.Join(dir, "projects/acme/specs/1.md"),
		"---\nid: 1\ntitle: S\nlabels: []\ntags: {}\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\nedits: []\n---\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"),
		"title: T\nstate: Accepted\nspec: 1\nlabels: []\ntags: {}\nblocked_by: []\nreplacements: []\n"+
			"created: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\n"+
			"pull_requests:\n  - repository: api\n    number: 1\n    link: l\n    state: open\n  - repository: api\n    number: 2\n    link: l\n    state: open\n"+
			"claims: []\nrounds: []\nfailure_reports: []\ncomments: []\nmoves: []\nedits: []\n")
	_, err := store.Open(dir)
	if err == nil || !strings.Contains(err.Error(), "pull_requests[1].repository") {
		t.Errorf("open = %v, want the second pull request in api refused", err)
	}
}
