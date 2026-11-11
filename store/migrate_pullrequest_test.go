package store_test

import (
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 24 added a Ticket's pull request and a pushed commit's branch.
// The real chain migrates a format-23 store: a Ticket file without them
// reads as having none, and one with them keeps them.
func TestTheRealChainMigratesAFormat23StoreToThePullRequestFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 23\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"), "key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	head := "state: Accepted\nspec: 1\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\n"
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"), "title: Plain\n"+head)
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/3.yaml"), "title: Opened\n"+head+"pull_request:\n  number: 7\n  link: https://github.com/a/b/pull/7\n  state: merged\n  merge_commit: abc123\n")

	s := openFile(t, dir)
	s.View(func(r store.Reader) error {
		if tk, _ := r.Ticket("acme", 2); tk.PullRequest != nil {
			t.Errorf("pull request of a plain Ticket = %+v, want none", tk.PullRequest)
		}
		tk, _ := r.Ticket("acme", 3)
		want := store.PullRequest{Number: 7, Link: "https://github.com/a/b/pull/7", State: store.PRMerged, MergeCommit: "abc123"}
		if tk.PullRequest == nil || *tk.PullRequest != want {
			t.Errorf("pull request = %+v, want %+v", tk.PullRequest, want)
		}
		return nil
	})
}
