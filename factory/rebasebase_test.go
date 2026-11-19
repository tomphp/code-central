package factory_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/factory/factorytest"
)

// Land per Spec off under Rebase (Spec code-central-1063): an accepted
// Ticket's own commits are replayed onto the base's tip, its branch moves
// to them, and the one `Merge <id>` commit then has the base's tip as its
// first parent and the replayed commits as its second.

// mergedLine is the commits a merge's second parent adds to its first
// parent's line, oldest first, and its merges among them.
func mergedLine(h *factorytest.Harness, dir, merge string) (commits, merges []string) {
	return strings.Fields(h.GitIn(dir, "rev-list", "--reverse", merge+"^1.."+merge+"^2")), strings.Fields(h.GitIn(dir, "rev-list", "--merges", merge+"^1.."+merge+"^2"))
}

func TestUnderRebaseWithoutLandPerSpecATicketIsReplayedOntoTheBaseBeforeItsMergeCommit(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	setRebase(h, true)
	id, commits := acceptedAs(t, h, "Add a feature", "feature")
	// The base moves on after the branch was cut.
	moved := h.CommitOn("main", "base.txt", "base\n")

	r := h.Start()
	tk := h.WantState(id, domain.Done)
	r.WantOutput(id + ": replayed onto main at ")

	merge := h.Git("rev-parse", "main")
	if got := subject(h, h.Repo, merge); got != "Merge "+id+": Add a feature" {
		t.Fatalf("main's tip is %q, want the Merge commit", got)
	}
	if m := tk.Moves[len(tk.Moves)-1]; m.Commit != merge || m.From != domain.Accepted {
		t.Errorf("land move %+v, want from Accepted with the merge %s", m, merge)
	}
	if got := h.Git("rev-parse", merge+"^1"); got != moved {
		t.Errorf("the merge's first parent is %s, want the base's tip %s", got, moved)
	}
	line, merges := mergedLine(h, h.Repo, merge)
	if len(line) != 2 || len(merges) > 0 {
		t.Fatalf("the merge adds %d commits with merges %v, want the 2 replayed and no merge", len(line), merges)
	}
	if got := h.Git("rev-parse", line[0]+"^"); got != moved {
		t.Errorf("the first replayed commit's parent is %s, want the base's tip %s", got, moved)
	}
	for i, orig := range commits {
		if line[i] == orig {
			t.Errorf("commit %d is the original %s, want it replayed onto the moved base", i, orig)
		}
		if got, want := authored(h, h.Repo, line[i]), authored(h, h.Repo, orig); got != want {
			t.Errorf("replayed commit %d is %q, want %q", i, got, want)
		}
	}
	if got := h.Git("rev-parse", id); got != line[1] {
		t.Errorf("%s's branch is at %s, want its replayed tip %s", id, got, line[1])
	}
}

func TestUnderRebaseWithoutLandPerSpecABranchOnTheBaseTipIsMergedAsItIs(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	setRebase(h, true)
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")
	oldMain := h.Git("rev-parse", "main")

	r := h.Start()
	h.WantState(id, domain.Done)

	merge := h.Git("rev-parse", "main")
	if got := h.Git("log", "-1", "--format=%P", merge); got != oldMain+" "+commit {
		t.Errorf("the merge's parents are %q, want the base's tip and the finish commit, %s %s", got, oldMain, commit)
	}
	if strings.Contains(r.Stdout(), id+": replayed onto") {
		t.Error("a branch already on the base's tip was replayed")
	}
}

func TestUnderRebaseWithoutLandPerSpecAFailedLandIsRetriedWithoutReplayingAgain(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	setRebase(h, true)
	id, _ := acceptedAs(t, h, "Add a feature", "feature")
	h.CommitOn("main", "base.txt", "base\n")
	h.SetFailing("/land")

	r := h.Start()
	r.WantOutput(id + ": land: ")
	merge := h.Git("rev-parse", "main")
	branch := h.Git("rev-parse", id)

	h.SetFailing("")
	r.Poll()
	h.WantState(id, domain.Done)
	if got := h.Git("rev-parse", "main"); got != merge {
		t.Errorf("main moved to %s on the retry, want it left at the merge %s", got, merge)
	}
	if got := h.Git("rev-parse", id); got != branch {
		t.Errorf("%s's branch moved to %s on the retry, want it left at %s", id, got, branch)
	}
	if got := strings.Fields(h.Git("rev-list", "--merges", "main")); len(got) != 1 {
		t.Errorf("main holds merges %v, want the one", got)
	}
}

func TestUnderRebaseWithoutLandPerSpecATicketConflictingInOneRepositoryIsReplayedIntoNeither(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	setRebase(h, true)
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)
	// Both bases move on: the Home Repository's cleanly, api's in conflict.
	homeBase := h.CommitOn("main", "base.txt", "base\n")
	apiBase := api.CommitOn("main", "endpoint.go", "package other\n")
	homeBranch, apiBranch := h.Git("rev-parse", id), h.GitIn(api.Path, "rev-parse", id)
	h.Script(id, factorytest.Wait())

	h.Accept(id)
	r.Poll()

	tk := h.Ticket(id)
	if !movedTo(tk, domain.Accepted, domain.Ready) {
		t.Fatalf("%s wasn't sent back: %+v", id, tk.Moves)
	}
	if h.Git("rev-parse", "main") != homeBase || h.GitIn(api.Path, "rev-parse", "main") != apiBase {
		t.Error("a base moved: a Ticket conflicting in any Repository lands in none")
	}
	if h.Git("rev-parse", id) != homeBranch || h.GitIn(api.Path, "rev-parse", id) != apiBranch {
		t.Error("a branch moved: a Ticket conflicting in any Repository is replayed onto none")
	}
	if !commented(tk, "replaying it onto the base conflicts in these Repositories, so it replayed it onto none of them", "- api: replaying", "onto main at "+apiBase[:12], "  - endpoint.go", "In each, rebase the branch onto its base") || commented(tk, "- "+h.Project+":") {
		t.Fatalf("comments %+v, want the send-back note to name api, its base and its file, and say to rebase", tk.Comments)
	}
}

func TestUnderRebaseWithoutLandPerSpecTwoRepositoriesAreBothReplayedAndLand(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddRepository("api")
	setRebase(h, true)
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)
	h.CommitOn("main", "base.txt", "base\n")
	api.CommitOn("main", "other.go", "package api\n")

	h.Accept(id)
	r.Poll()

	h.WantState(id, domain.Done)
	for _, dir := range []string{h.Repo, api.Path} {
		merge := h.GitIn(dir, "rev-parse", "main")
		line, merges := mergedLine(h, dir, merge)
		if len(line) != 1 || len(merges) > 0 {
			t.Errorf("the merge in %s adds %d commits with merges %v, want the one replayed", dir, len(line), merges)
			continue
		}
		if got := h.GitIn(dir, "rev-parse", line[0]+"^"); got != h.GitIn(dir, "rev-parse", merge+"^1") {
			t.Errorf("the replayed commit in %s is on %s, want the base's old tip", dir, got)
		}
		if got := h.GitIn(dir, "rev-parse", id); got != line[0] {
			t.Errorf("%s's branch in %s is at %s, want its replayed tip %s", id, dir, got, line[0])
		}
	}
}
