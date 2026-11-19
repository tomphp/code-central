package factory_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/factory/factorytest"
)

// Landing a Spec under Rebase (Spec code-central-1063): a Spec Branch the
// base has moved on from is rebased onto the base's tip, every Repository
// checked first, before the one `Merge spec` commit, whose second parent
// is the rebased Spec Branch.

// specLine is the subjects of the commits on dir's Spec Branch's
// first-parent line that rev lacks, oldest first, and its merge commits'
// subjects among them.
func specLine(h *factorytest.Harness, dir, rev string) (subjects, merges []string) {
	spec := "spec/" + h.SpecID()
	for _, c := range strings.Fields(h.GitIn(dir, "rev-list", "--first-parent", "--reverse", rev+".."+spec)) {
		subjects = append(subjects, subject(h, dir, c))
	}
	for _, c := range strings.Fields(h.GitIn(dir, "rev-list", "--first-parent", "--merges", rev+".."+spec)) {
		merges = append(merges, subject(h, dir, c))
	}
	return subjects, merges
}

// mergeSpecs is the `Merge spec` commits on dir's main that rev lacks.
func mergeSpecs(h *factorytest.Harness, dir, rev string) []string {
	var out []string
	for _, c := range strings.Fields(h.GitIn(dir, "rev-list", "--first-parent", rev+"..main")) {
		if strings.HasPrefix(subject(h, dir, c), "Merge spec ") {
			out = append(out, c)
		}
	}
	return out
}

func TestUnderRebaseASpecBranchIsRebasedOntoAMovedBaseBeforeItsOneMergeSpecCommit(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	setRebase(h, true)
	first, firstCommits := acceptedAs(t, h, "First", "one")
	second, secondCommits := acceptedAs(t, h, "Second", "two")
	release := holdTicketsStage(t, h)
	r := h.Start()
	r.WantOutput(second + ": replayed onto spec/" + h.SpecID())

	moved := h.CommitOn("main", "base.txt", "base\n")
	release()
	r.Poll()
	h.WantState(first, domain.Done)
	h.WantState(second, domain.Done)

	spec := "spec/" + h.SpecID()
	tip := h.Git("rev-parse", spec)
	if got := h.Git("log", "-1", "--format=%P", "main"); got != moved+" "+tip {
		t.Fatalf("main's tip has parents %q, want the moved base %s then the rebased Spec Branch %s", got, moved, tip)
	}
	if got := subject(h, h.Repo, "main"); got != "Merge spec "+h.SpecID()+": Work" {
		t.Errorf("main's tip is %q, want the Spec's merge", got)
	}
	if got := mergeSpecs(h, h.Repo, moved); len(got) != 1 {
		t.Errorf("main holds %d Merge spec commits, want exactly one", len(got))
	}
	commits := strings.Fields(h.Git("rev-list", "--reverse", moved+".."+spec))
	if merges := h.Git("rev-list", "--merges", moved+".."+spec); merges != "" || len(commits) != 4 {
		t.Fatalf("the Spec Branch holds %d commits on the moved base with merges %q, want the 4 rebased and no merge", len(commits), merges)
	}
	if got := h.Git("log", "-1", "--format=%P", commits[0]); got != moved {
		t.Errorf("the Spec Branch's first commit is on %s, want the moved base %s", got, moved)
	}
	for i, orig := range append(firstCommits, secondCommits...) {
		if got, want := authored(h, h.Repo, commits[i]), authored(h, h.Repo, orig); got != want {
			t.Errorf("rebased commit %d is %q, want %q", i, got, want)
		}
	}
	r.WantOutput(h.SpecID() + ": rebased " + spec + " onto main at " + tip[:12])
	if m := h.Ticket(second).Moves; m[len(m)-1].Commit != h.Git("rev-parse", "main") {
		t.Errorf("%s landed at %q, want the Merge spec commit", second, m[len(m)-1].Commit)
	}
}

func TestUnderRebaseAMixedSpecBranchKeepsItsEarlierMergesAsMergesWhenRebasedOntoAMovedBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	release := holdTicketsStage(t, h)
	first, _ := accepted(h, "First", "one.txt", "1\n")
	firstHead := h.Git("rev-parse", first)
	r := h.Start()
	r.WantOutput(first + ": merged into spec/" + h.SpecID())
	setRebase(h, true)
	second, _ := accepted(h, "Second", "two.txt", "2\n")
	r.Poll()
	r.WantOutput(second + ": replayed onto spec/" + h.SpecID())

	moved := h.CommitOn("main", "base.txt", "base\n")
	release()
	r.Poll()
	h.WantState(first, domain.Done)
	h.WantState(second, domain.Done)

	spec := "spec/" + h.SpecID()
	subjects, merges := specLine(h, h.Repo, moved)
	if got, want := strings.Join(subjects, ", "), "Merge "+first+": First, Change two.txt"; got != want {
		t.Fatalf("the Spec Branch's line on the moved base is %q, want %q", got, want)
	}
	if len(merges) != 1 {
		t.Fatalf("the Spec Branch's line holds merges %q, want the earlier Merge commit kept as one", merges)
	}
	remade := strings.Fields(h.Git("rev-list", "--first-parent", "--reverse", moved+".."+spec))[0]
	if got := h.Git("log", "-1", "--format=%P", remade); got != moved+" "+firstHead {
		t.Errorf("the remade merge has parents %q, want the moved base %s and %s's head %s", got, moved, first, firstHead)
	}
	if got := h.Git("log", "-1", "--format=%P", "main"); got != moved+" "+h.Git("rev-parse", spec) {
		t.Errorf("main's tip has parents %q, want the moved base then the rebased Spec Branch", got)
	}
	if got := mergeSpecs(h, h.Repo, moved); len(got) != 1 {
		t.Errorf("main holds %d Merge spec commits, want exactly one", len(got))
	}
}

func TestUnderRebaseASpecBranchConflictingInOneRepositoryRebasesAndLandsNothingAnywhere(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api, id := specInTwo(h)
	setRebase(h, true)
	release := holdTicketsStage(t, h)
	fix := "acme-5"
	h.Script(fix, factorytest.Wait())
	r := h.Start()
	h.WantState(id, domain.InReview)
	h.Accept(id)
	r.Poll()
	r.WantOutput(id + ": replayed onto spec/" + h.SpecID())
	spec := "spec/" + h.SpecID()
	homeTip, apiTip := h.Git("rev-parse", spec), h.GitIn(api.Path, "rev-parse", spec)
	// The Home Repository's base moves on cleanly; api's conflicts.
	homeBase := h.CommitOn("main", "base.txt", "base\n")
	apiBase := api.CommitOn("main", "endpoint.go", "package other\n")

	release()
	r.Poll()

	h.WantState(id, domain.Accepted)
	if h.Git("rev-parse", "main") != homeBase || h.GitIn(api.Path, "rev-parse", "main") != apiBase {
		t.Fatal("a base moved: nothing lands while any Repository's Spec Branch conflicts")
	}
	if h.Git("rev-parse", spec) != homeTip || h.GitIn(api.Path, "rev-parse", spec) != apiTip {
		t.Fatal("a Spec Branch moved: none is rebased while any Repository's conflicts")
	}
	tk := h.Ticket(fix)
	if !tk.SpecMergeTicket || !strings.Contains(tk.Description, "- api: ") || !strings.Contains(tk.Description, "  - endpoint.go\n") || strings.Contains(tk.Description, "- "+h.Project+": ") {
		t.Fatalf("%s %+v, want the Spec merge Ticket naming api's endpoint.go alone", fix, tk)
	}
	r.WantOutput(h.SpecID() + ": " + spec + " conflicts with the base in api: opened " + fix)
}

func TestUnderMergeAndPushTheBaseMoveStillRebuildsAfterARebaseLanding(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	on := true
	h.SetLandingMethod(core.SetLandingMethodInput{Method: domain.LandingMerge, Push: true, Remote: "origin", Rebase: &on})
	h.Git("push", "-q", remote.Path, "main:refs/heads/main")
	h.MakeRequirement()
	a, _ := accepted(h, "First", "one.txt", "1\n")
	b, _ := accepted(h, "Second", "two.txt", "2\n")
	release := holdTicketsStage(t, h)
	r := h.Start()
	r.WantOutput(b + ": replayed onto spec/" + h.SpecID())

	// The base moves on, pushed, so the Spec Branch is rebased at landing.
	moved := h.CommitOn("main", "base.txt", "base\n")
	h.Git("push", "-q", remote.Path, "main:refs/heads/main")
	remote.RefusePushes()
	release()
	r.Poll()
	spec := "spec/" + h.SpecID()
	r.WantOutput(h.SpecID() + ": rebased " + spec + " onto main at ")
	r.WantOutput(a + ": can't land yet")
	specTip := h.Git("rev-parse", spec)
	h.Git("merge-base", "--is-ancestor", moved, specTip)

	// Meanwhile the remote's base moves on too.
	h.Git("branch", "theirs", moved)
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	remote.AcceptPushes()
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")
	r.Poll()
	r.WantOutput("main: moved 1 unpushed commit onto origin/main")
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
	if got := h.Git("log", "-1", "--format=%P", "main"); got != theirs+" "+specTip {
		t.Errorf("main's tip has parents %q, want the remote's %s then the rebased Spec Branch's %s", got, theirs, specTip)
	}
	if got := remote.Git("rev-parse", "main"); got != h.Git("rev-parse", "main") {
		t.Errorf("the remote's main is at %q, want it pushed level with the local base", got)
	}
}
