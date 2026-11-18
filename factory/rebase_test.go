package factory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/factory/factorytest"
)

// Replaying an accepted Ticket onto its Spec Branch under Rebase (Spec
// code-central-1063): its own commits go onto the Spec Branch as they
// are, with no `Merge <id>` commit, each with its author and message, and
// never twice.

// setRebase turns the Project's Rebase on or off, under Merge.
func setRebase(h *factorytest.Harness, on bool) {
	h.SetLandingMethod(core.SetLandingMethodInput{Method: domain.LandingMerge, Rebase: &on})
}

// commitAs commits file with content on branch, made from main when it
// doesn't exist, by author at date with message, and returns the commit.
// The repo is left on main.
func commitAs(t *testing.T, h *factorytest.Harness, dir, branch, file, content, author, date, message string) string {
	t.Helper()
	if !strings.Contains(h.GitIn(dir, "branch", "--list", branch), branch) {
		h.GitIn(dir, "branch", branch, "main")
	}
	h.GitIn(dir, "checkout", "-q", branch)
	defer h.GitIn(dir, "checkout", "-q", "main")
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	h.GitIn(dir, "add", file)
	h.GitIn(dir, "commit", "-q", "--author", author, "--date", date, "-m", message)
	return h.GitIn(dir, "rev-parse", "HEAD")
}

// authored is commit's author, author date and message, as a replay keeps
// them.
func authored(h *factorytest.Harness, dir, commit string) string {
	return h.GitIn(dir, "log", "-1", "--format=%an <%ae> %aI%n%B", commit)
}

// onSpecBranch is the commits on the Spec Branch that main lacks, oldest
// first, and its merge commits among them.
func onSpecBranch(h *factorytest.Harness, dir string) (commits, merges []string) {
	spec := "spec/" + h.SpecID()
	return strings.Fields(h.GitIn(dir, "rev-list", "--reverse", "main.."+spec)), strings.Fields(h.GitIn(dir, "rev-list", "--merges", "main.."+spec))
}

// acceptedAs makes an Accepted Ticket whose branch, cut from main, holds
// two commits by different authors, and returns it with them.
func acceptedAs(t *testing.T, h *factorytest.Harness, title, file string) (id string, commits []string) {
	t.Helper()
	id = h.ReadyTicket(title)
	commits = append(commits,
		commitAs(t, h, h.Repo, id, file+".a", "a\n", "Ada Lovelace <ada@example.com>", "2001-02-03T04:05:06+00:00", "Start "+title+"\n\nWhy it starts here."),
		commitAs(t, h, h.Repo, id, file+".b", "b\n", "Grace Hopper <grace@example.com>", "2002-03-04T05:06:07+00:00", "Finish "+title))
	h.Finished(id, id, commits[1])
	h.Accept(id)
	return id, commits
}

func TestUnderRebaseAcceptedTicketsAreReplayedOntoALinearSpecBranchKeepingAuthorsAndMessages(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	setRebase(h, true)
	// Both cut from main, so the second is replayed onto the first's.
	first, firstCommits := acceptedAs(t, h, "First", "one")
	second, secondCommits := acceptedAs(t, h, "Second", "two")
	oldMain := h.Git("rev-parse", "main")
	holdTicketsStage(t, h)

	r := h.Start()
	r.WantOutput(first + ": replayed onto spec/" + h.SpecID() + " at ")
	r.WantOutput(second + ": replayed onto spec/" + h.SpecID() + " at ")

	commits, merges := onSpecBranch(h, h.Repo)
	if len(merges) > 0 || len(commits) != 4 {
		t.Fatalf("the Spec Branch holds %d commits with merges %v, want the 4 replayed and no merge", len(commits), merges)
	}
	for i, orig := range append(firstCommits, secondCommits...) {
		if got, want := authored(h, h.Repo, commits[i]), authored(h, h.Repo, orig); got != want {
			t.Errorf("replayed commit %d is %q, want %q", i, got, want)
		}
	}
	spec := "spec/" + h.SpecID()
	if got := h.Git("log", "-1", "--format=%P", commits[0]); got != oldMain {
		t.Errorf("the first replayed commit's parent is %q, want main's tip %s", got, oldMain)
	}
	tip := h.Git("rev-parse", spec)
	if got := h.Git("rev-parse", second); got != tip {
		t.Errorf("%s's branch is at %s, want the Spec Branch's tip %s", second, got, tip)
	}
	if got := h.Git("rev-parse", first); got != commits[1] {
		t.Errorf("%s's branch is at %s, want its replayed tip %s", first, got, commits[1])
	}
	if h.Git("rev-parse", "main") != oldMain {
		t.Error("main moved: nothing reaches the base until the Spec lands")
	}
	tk := h.Ticket(second)
	if tk.State != domain.Accepted || tk.SpecMerge == nil || tk.SpecMerge.Commit != tip || !tk.SpecMerge.Counts {
		t.Errorf("%s is %s with Spec merge %+v, want Accepted merged at %s", second, tk.State, tk.SpecMerge, tip)
	}
}

func TestUnderRebaseAReplayIsNeverMadeTwiceAfterAFailedReportOrARestart(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	setRebase(h, true)
	id, _ := acceptedAs(t, h, "Add a feature", "feature")
	holdTicketsStage(t, h)
	h.SetFailing("/report_spec_merge")

	r := h.Start()
	r.WantOutput(id + ": report_spec_merge: ")
	spec := "spec/" + h.SpecID()
	tip := h.Git("rev-parse", spec)
	if commits, _ := onSpecBranch(h, h.Repo); len(commits) != 2 {
		t.Fatalf("the Spec Branch holds %d commits, want the 2 replayed", len(commits))
	}

	h.SetFailing("")
	r.Poll()
	tk := h.Ticket(id)
	if tk.SpecMerge == nil || tk.SpecMerge.Commit != tip || !tk.SpecMerge.Counts {
		t.Fatalf("Spec merge %+v, want it reported at %s", tk.SpecMerge, tip)
	}
	if got := h.Git("rev-parse", spec); got != tip {
		t.Fatalf("the Spec Branch moved to %s on the retry, want it left at %s", got, tip)
	}

	r.Stop()
	r = h.Start()
	r.Poll()
	if got := h.Git("rev-parse", spec); got != tip {
		t.Errorf("the Spec Branch moved to %s after a restart, want it left at %s", got, tip)
	}
}

func TestUnderRebaseATicketCutBeforeItsSpecBranchWasRewrittenReplaysOnlyItsOwnCommits(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	setRebase(h, true)
	first, _ := accepted(h, "First", "one.txt", "1\n")
	holdTicketsStage(t, h)
	r := h.Start()
	r.WantOutput(first + ": replayed onto spec/" + h.SpecID())

	// Cut from the Spec Branch holding the first Ticket's commit.
	second := h.ReadyTicket("Second")
	h.Script(second, factorytest.WriteFile("two.txt", "2\n"), factorytest.Git("add", "two.txt"), factorytest.Git("commit", "-q", "-m", "Add two"),
		factorytest.CLI("finish", "{{ticket}}", "--claim", "{{claim}}", "--summary", "Done.", "--branch", "{{ticket}}", "--commit", "{{head}}"), factorytest.Result("success"))
	r.Poll()
	h.WantState(second, domain.InReview)

	// The Spec Branch is rewritten under it: rebased onto a moved base.
	h.CommitOn("main", "base.txt", "base\n")
	spec := "spec/" + h.SpecID()
	h.Git("checkout", "-q", spec)
	h.Git("rebase", "-q", "main")
	h.Git("checkout", "-q", "main")
	rewritten := h.Git("rev-parse", spec)

	h.Accept(second)
	r.Poll()

	commits, merges := onSpecBranch(h, h.Repo)
	var subjects []string
	for _, c := range commits {
		subjects = append(subjects, subject(h, h.Repo, c))
	}
	if len(merges) > 0 || strings.Join(subjects, ", ") != "Change one.txt, Add two" {
		t.Fatalf("the Spec Branch holds %q with merges %v, want the first Ticket's commit once and then the second's own", subjects, merges)
	}
	if got := h.Git("log", "-1", "--format=%P", commits[1]); got != rewritten {
		t.Errorf("the second Ticket's commit is on %s, want the rewritten Spec Branch's tip %s", got, rewritten)
	}
	if tk := h.Ticket(first); tk.State != domain.Accepted {
		t.Errorf("%s is %s, want it still Accepted, waiting for the Spec", first, tk.State)
	}
}

func TestUnderRebaseASpecBranchConflictSendsTheTicketBackToRebase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	setRebase(h, true)
	first, _ := accepted(h, "First", "README", "first\n")
	second, _ := accepted(h, "Second", "README", "second\n")
	spec := "spec/" + h.SpecID()

	h.Script(second, factorytest.Wait())
	r := h.Start()
	r.WantOutput(first + ": replayed onto " + spec)
	tk := h.Ticket(second)
	if !movedTo(tk, domain.Accepted, domain.Ready) {
		t.Fatalf("%s was not sent back: %+v", second, tk.Moves)
	}
	if !commented(tk, "replaying", "onto "+spec, "- README", "Rebase the branch onto "+spec+", resolve the conflicts, and finish again.") || commented(tk, "Merge "+spec+" into the branch") {
		t.Errorf("comments %+v, want the send-back note to say to rebase onto the Spec Branch", tk.Comments)
	}
	if commits, merges := onSpecBranch(h, h.Repo); len(commits) != 1 || len(merges) > 0 {
		t.Errorf("the Spec Branch holds %d commits with merges %v, want the first Ticket's alone", len(commits), merges)
	}
}

func TestUnderRebaseABaseConflictSendsBackAndClaimsWithTheRebaseWording(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	setRebase(h, true)
	id, _ := accepted(h, "Change the readme", "README", "theirs\n")
	h.CommitOn("main", "README", "ours\n")

	h.Script(id, factorytest.Wait())
	h.Start()

	// Not Spec-landed, it was merged into the base, not replayed, but is
	// asked to rebase all the same.
	if !commented(h.Ticket(id), "merging", "into main", "Rebase the branch onto main, resolve the conflicts, and finish again.") || commented(h.Ticket(id), "replaying") {
		t.Errorf("comments %+v, want the send-back note to say the merge conflicted and to rebase onto main", h.Ticket(id).Comments)
	}
	c := callFor(t, h, id)
	requireConflictClaim(t, c, "README")
	if !strings.Contains(c.Prompt, "Rebase the branch onto main and resolve the conflicts") || strings.Contains(c.Prompt, "Merge main into the branch") {
		t.Errorf("first prompt %q, want it to say to rebase onto main", c.Prompt)
	}
}

func TestTurningRebaseOnPartWayLeavesEarlierMergesAndRebaseOffMergesLaterTickets(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	release := holdTicketsStage(t, h)
	first, _ := accepted(h, "First", "one.txt", "1\n")
	r := h.Start()
	r.WantOutput(first + ": merged into spec/" + h.SpecID())

	setRebase(h, true)
	second, _ := accepted(h, "Second", "two.txt", "2\n")
	r.Poll()
	r.WantOutput(second + ": replayed onto spec/" + h.SpecID())

	setRebase(h, false)
	third, _ := accepted(h, "Third", "three.txt", "3\n")
	r.Poll()
	r.WantOutput(third + ": merged into spec/" + h.SpecID())

	spec := "spec/" + h.SpecID()
	var subjects []string
	for _, c := range strings.Fields(h.Git("rev-list", "--first-parent", "--reverse", "main.."+spec)) {
		subjects = append(subjects, subject(h, h.Repo, c))
	}
	want := "Merge " + first + ": First, Change two.txt, Merge " + third + ": Third"
	if got := strings.Join(subjects, ", "); got != want {
		t.Fatalf("the Spec Branch's line is %q, want %q", got, want)
	}

	// The mixed Spec Branch still lands.
	release()
	r.Poll()
	h.WantState(first, domain.Done)
	h.WantState(second, domain.Done)
	h.WantState(third, domain.Done)
}

func TestUnderRebaseTwoRepositoriesAreBothReplayedAndTheFinishCheckStillPasses(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api, id := specInTwo(h)
	setRebase(h, true)
	holdTicketsStage(t, h)
	r := h.Start()
	h.WantState(id, domain.InReview)
	h.SetFailing("/report_spec_merge")

	h.Accept(id)
	r.Poll()
	r.WantOutput(id + ": report_spec_merge: ")
	h.SetFailing("")
	r.Poll()

	tk := h.Ticket(id)
	if tk.State != domain.Accepted || tk.SpecMerge == nil || !tk.SpecMerge.Counts {
		t.Fatalf("%s is %s with Spec merge %+v, want Accepted with it reported: the finish check passed the replayed branches", id, tk.State, tk.SpecMerge)
	}
	for _, dir := range []string{h.Repo, api.Path} {
		commits, merges := onSpecBranch(h, dir)
		if len(commits) != 1 || len(merges) > 0 {
			t.Errorf("the Spec Branch in %s holds %d commits with merges %v, want the one replayed", dir, len(commits), merges)
		}
		if got, want := h.GitIn(dir, "rev-parse", id), h.GitIn(dir, "rev-parse", "spec/"+h.SpecID()); got != want {
			t.Errorf("%s's branch in %s is at %s, want the Spec Branch's tip %s", id, dir, got, want)
		}
	}
}

func TestUnderRebaseATicketConflictingInOneRepositoryIsReplayedIntoNeither(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api, first := specInTwo(h)
	setRebase(h, true)
	release := holdTicketsStage(t, h)
	defer release()
	// Cut from the same Spec Branch tips, it writes api's endpoint.go too.
	second := h.ReadyTicket("Change the endpoint")
	h.Script(second, append(factorytest.CommitIn("api", "endpoint.go", "package other\n"),
		factorytest.WriteFile("other.go", "package acme\n"), factorytest.Git("add", "other.go"), factorytest.Git("commit", "-q", "-m", "Other"),
		factorytest.FinishIn("Done.", true, "api"), factorytest.Result("success"))...)
	r := h.Start()
	h.WantState(first, domain.InReview)
	h.WantState(second, domain.InReview)
	h.Accept(first)
	r.Poll()
	spec := "spec/" + h.SpecID()
	homeTip, apiTip := h.Git("rev-parse", spec), h.GitIn(api.Path, "rev-parse", spec)
	homeBranch := h.Git("rev-parse", second)
	h.Script(second, factorytest.Wait())

	h.Accept(second)
	r.Poll()

	tk := h.Ticket(second)
	if !movedTo(tk, domain.Accepted, domain.Ready) {
		t.Fatalf("%s wasn't sent back: %+v", second, tk.Moves)
	}
	if h.Git("rev-parse", spec) != homeTip || h.GitIn(api.Path, "rev-parse", spec) != apiTip {
		t.Fatal("a Spec Branch moved: a Ticket conflicting in any Repository is replayed into none")
	}
	if h.Git("rev-parse", second) != homeBranch {
		t.Error("the Home Repository's branch moved: a Ticket conflicting in any Repository is replayed into none")
	}
	if !commented(tk, "replaying it onto the Spec Branch conflicts in these Repositories, so it replayed it onto none of them", "- api: replaying", "onto "+spec+" at "+apiTip[:12], "  - endpoint.go", "In each, rebase the branch onto its Spec Branch") {
		t.Fatalf("comments %+v, want the send-back note to name api's Spec Branch and file and say to rebase", tk.Comments)
	}
}

func TestUnderRebaseADirtyWorktreeHoldsTheReplayAndIsLeftAlone(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	setRebase(h, true)
	id, commits := acceptedAs(t, h, "Add a feature", "feature")
	// The base moves on, so the replay makes new commits.
	h.CommitOn("main", "base.txt", "base\n")
	h.Git("worktree", "add", "-q", h.Worktree(id), id)
	mine := filepath.Join(h.Worktree(id), "scratch.txt")
	if err := os.WriteFile(mine, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	holdTicketsStage(t, h)

	r := h.Start()
	r.WantOutput(id + ": can't land yet")
	if got := h.Git("rev-parse", id); got != commits[1] {
		t.Errorf("the branch moved to %s, want it left at %s", got, commits[1])
	}
	if c, _ := onSpecBranch(h, h.Repo); len(c) != 0 {
		t.Errorf("the Spec Branch holds %d commits, want none while the worktree is in the way", len(c))
	}

	if err := os.Remove(mine); err != nil {
		t.Fatal(err)
	}
	r.Poll()
	// Reset, not moved under it, the worktree is clean, so the sweep
	// removes it.
	r.WantOutput(id + ": Accepted: removed its worktree")
	tip := h.Git("rev-parse", "spec/"+h.SpecID())
	if got := h.Git("rev-parse", id); got != tip || got == commits[1] {
		t.Errorf("the branch is at %s, want it moved from %s to the Spec Branch's tip %s", got, commits[1], tip)
	}
}

func TestUnderRebaseACommitAlreadyOnTheSpecBranchIsDroppedNotReplayedEmpty(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	setRebase(h, true)
	first, _ := accepted(h, "First", "one.txt", "1\n")
	second := h.ReadyTicket("Second")
	commitAs(t, h, h.Repo, second, "one.txt", "1\n", "Ada Lovelace <ada@example.com>", "2001-02-03T04:05:06+00:00", "Make the same change")
	head := commitAs(t, h, h.Repo, second, "two.txt", "2\n", "Ada Lovelace <ada@example.com>", "2001-02-03T04:05:07+00:00", "Add two")
	h.Finished(second, second, head)
	h.Accept(second)
	holdTicketsStage(t, h)

	r := h.Start()
	r.WantOutput(first + ": replayed onto spec/" + h.SpecID())
	r.WantOutput(second + ": replayed onto spec/" + h.SpecID())
	commits, merges := onSpecBranch(h, h.Repo)
	var subjects []string
	for _, c := range commits {
		subjects = append(subjects, subject(h, h.Repo, c))
	}
	if len(merges) > 0 || strings.Join(subjects, ", ") != "Change one.txt, Add two" {
		t.Fatalf("the Spec Branch holds %q with merges %v, want the change already on it dropped, as git rebase drops it", subjects, merges)
	}
}

func TestUnderRebaseABranchMovedOnFromItsFinishCommitIsHeldNotMerged(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	setRebase(h, true)
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	h.CommitOn(id, "later.txt", "later\n")
	holdTicketsStage(t, h)

	r := h.Start()
	r.WantOutput(id + ": can't land yet")
	if !commented(h.Ticket(id), "isn't at its finish commit", "finish again at the branch's head") {
		t.Errorf("comments %+v, want a can't-land note on the moved branch", h.Ticket(id).Comments)
	}
	if commits, merges := onSpecBranch(h, h.Repo); len(commits) != 0 || len(merges) != 0 {
		t.Errorf("the Spec Branch holds %v, merges %v: want nothing, and no Merge commit under Rebase", commits, merges)
	}
}
