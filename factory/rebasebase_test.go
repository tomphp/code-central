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
func mergedLine(v gitView, merge string) (commits, merges []string) {
	parents := v.Parents(merge)
	return inRange(v, parents[1], parents[0])
}

func TestUnderRebaseWithoutLandPerSpecATicketIsReplayedOntoTheBaseBeforeItsMergeCommit(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	setRebase(h, true)
	id, commits := acceptedTwice(h, "Add a feature", "feature")
	// The base moves on after the branch was cut.
	moved := h.CommitOn("main", "base.txt", "base\n")

	r := h.Start()
	tk := h.WantState(id, domain.Done)
	r.WantOutput(id + ": replayed onto main at ")

	merge := h.Tip("main")
	if got := h.Subject(merge); got != "Merge "+id+": Add a feature" {
		t.Fatalf("main's tip is %q, want the Merge commit", got)
	}
	if m := tk.Moves[len(tk.Moves)-1]; m.Commit != merge || m.From != domain.Accepted {
		t.Errorf("land move %+v, want from Accepted with the merge %s", m, merge)
	}
	if got := h.Parents(merge)[0]; got != moved {
		t.Errorf("the merge's first parent is %s, want the base's tip %s", got, moved)
	}
	line, merges := mergedLine(h, merge)
	if len(line) != 2 || len(merges) > 0 {
		t.Fatalf("the merge adds %d commits with merges %v, want the 2 replayed and no merge", len(line), merges)
	}
	if got := h.Parents(line[0]); len(got) != 1 || got[0] != moved {
		t.Errorf("the first replayed commit's parents are %v, want the base's tip %s", got, moved)
	}
	for i, orig := range commits {
		if line[i] == orig {
			t.Errorf("commit %d is the original %s, want it replayed onto the moved base", i, orig)
		}
		if got, want := h.Subject(line[i]), h.Subject(orig); got != want {
			t.Errorf("replayed commit %d is %q, want %q", i, got, want)
		}
	}
	if got := h.Tip(id); got != line[1] {
		t.Errorf("%s's branch is at %s, want its replayed tip %s", id, got, line[1])
	}
}

func TestUnderRebaseWithoutLandPerSpecABranchOnTheBaseTipIsMergedAsItIs(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	setRebase(h, true)
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")
	oldMain := h.Tip("main")

	r := h.Start()
	h.WantState(id, domain.Done)

	merge := h.Tip("main")
	if got := strings.Join(h.Parents(merge), " "); got != oldMain+" "+commit {
		t.Errorf("the merge's parents are %q, want the base's tip and the finish commit, %s %s", got, oldMain, commit)
	}
	if strings.Contains(r.Stdout(), id+": replayed onto") {
		t.Error("a branch already on the base's tip was replayed")
	}
}

func TestUnderRebaseWithoutLandPerSpecAFailedLandIsRetriedWithoutReplayingAgain(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	setRebase(h, true)
	id, _ := acceptedTwice(h, "Add a feature", "feature")
	base := h.CommitOn("main", "base.txt", "base\n")
	h.SetFailing("/land")

	r := h.Start()
	r.WantOutput(id + ": land: ")
	merge := h.Tip("main")
	branch := h.Tip(id)

	h.SetFailing("")
	r.Poll()
	h.WantState(id, domain.Done)
	if got := h.Tip("main"); got != merge {
		t.Errorf("main moved to %s on the retry, want it left at the merge %s", got, merge)
	}
	if got := h.Tip(id); got != branch {
		t.Errorf("%s's branch moved to %s on the retry, want it left at %s", id, got, branch)
	}
	if _, merges := inRange(h, merge, base); len(merges) != 1 {
		t.Errorf("main holds merges %v, want the one", merges)
	}
}

func TestUnderRebaseWithoutLandPerSpecATicketConflictingInOneRepositoryIsReplayedIntoNeither(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
	api := h.AddRepository("api")
	setRebase(h, true)
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)
	// Both bases move on: the Home Repository's cleanly, api's in conflict.
	homeBase := h.CommitOn("main", "base.txt", "base\n")
	apiBase := api.CommitOn("main", "endpoint.go", "package other\n")
	homeBranch, apiBranch := h.Tip(id), api.Tip(id)
	h.Script(id, factorytest.Wait())

	h.Accept(id)
	r.Poll()

	tk := h.Ticket(id)
	if !movedTo(tk, domain.Accepted, domain.Ready) {
		t.Fatalf("%s wasn't sent back: %+v", id, tk.Moves)
	}
	if h.Tip("main") != homeBase || api.Tip("main") != apiBase {
		t.Error("a base moved: a Ticket conflicting in any Repository lands in none")
	}
	if h.Tip(id) != homeBranch || api.Tip(id) != apiBranch {
		t.Error("a branch moved: a Ticket conflicting in any Repository is replayed onto none")
	}
	if !commented(tk, "replaying it onto the base conflicts in these Repositories, so it replayed it onto none of them", "- api: replaying", "onto main at "+apiBase[:12], "  - endpoint.go", "In each, rebase the branch onto its base") || commented(tk, "- "+h.Project+":") {
		t.Fatalf("comments %+v, want the send-back note to name api, its base and its file, and say to rebase", tk.Comments)
	}
}

func TestUnderRebaseWithoutLandPerSpecTwoRepositoriesAreBothReplayedAndLand(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
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
	for name, v := range map[string]gitView{h.Project: h, "api": api} {
		merge := v.Tip("main")
		line, merges := mergedLine(v, merge)
		if len(line) != 1 || len(merges) > 0 {
			t.Errorf("the merge in %s adds %d commits with merges %v, want the one replayed", name, len(line), merges)
			continue
		}
		if got := v.Parents(line[0]); len(got) != 1 || got[0] != v.Parents(merge)[0] {
			t.Errorf("the replayed commit in %s is on %v, want the base's old tip", name, got)
		}
		if got := v.Tip(id); got != line[0] {
			t.Errorf("%s's branch in %s is at %s, want its replayed tip %s", id, name, got, line[0])
		}
	}
}

func TestUnderRebaseWithoutLandPerSpecWorkAlreadyInTheBaseLandsAtItsTipWithNoMerge(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	setRebase(h, true)
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	// The same change reaches the base by another commit.
	tip := h.CommitOn("main", "feature.txt", "feature\n")

	h.Start()
	tk := h.WantState(id, domain.Done)

	if got := h.Tip("main"); got != tip {
		t.Errorf("main moved to %s, want it left at %s: nothing is left to merge", got, tip)
	}
	if m := tk.Moves[len(tk.Moves)-1]; m.Commit != tip {
		t.Errorf("land move %+v, want it landed at the base's tip %s", m, tip)
	}
	if got := h.Tip(id); got != tip {
		t.Errorf("%s's branch is at %s, want it moved to the base's tip %s", id, got, tip)
	}
}
