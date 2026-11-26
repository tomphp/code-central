package factory_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
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
func specLine(h *factorytest.Rig, v gitView, rev string) (subs, merges []string) {
	commits, mergeCommits := inRange(v, v.Tip("spec/"+h.SpecID()), rev)
	return subjects(v, commits), subjects(v, mergeCommits)
}

// mergeSpecs is the `Merge spec` commits on v's main that rev lacks.
func mergeSpecs(v gitView, rev string) []string {
	var out []string
	commits, _ := inRange(v, v.Tip("main"), rev)
	for _, c := range commits {
		if strings.HasPrefix(v.Subject(c), "Merge spec ") {
			out = append(out, c)
		}
	}
	return out
}

func TestUnderRebaseASpecBranchIsRebasedOntoAMovedBaseBeforeItsOneMergeSpecCommit(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.MakeRequirement()
	setRebase(h, true)
	first, firstCommits := acceptedTwice(h, "First", "one")
	second, secondCommits := acceptedTwice(h, "Second", "two")
	release := holdTicketsStage(t, h)
	r := h.Start()
	r.WantOutput(second + ": replayed onto spec/" + h.SpecID())

	moved := h.CommitOn("main", "base.txt", "base\n")
	release()
	r.Poll()
	h.WantState(first, domain.Done)
	h.WantState(second, domain.Done)

	spec := "spec/" + h.SpecID()
	tip := h.Tip(spec)
	if got := strings.Join(h.Parents(h.Tip("main")), " "); got != moved+" "+tip {
		t.Fatalf("main's tip has parents %q, want the moved base %s then the rebased Spec Branch %s", got, moved, tip)
	}
	if got := h.Subject("main"); got != "Merge spec "+h.SpecID()+": Work" {
		t.Errorf("main's tip is %q, want the Spec's merge", got)
	}
	if got := mergeSpecs(h, moved); len(got) != 1 {
		t.Errorf("main holds %d Merge spec commits, want exactly one", len(got))
	}
	commits, merges := inRange(h, tip, moved)
	if len(merges) != 0 || len(commits) != 4 {
		t.Fatalf("the Spec Branch holds %d commits on the moved base with merges %q, want the 4 rebased and no merge", len(commits), merges)
	}
	if got := h.Parents(commits[0]); len(got) != 1 || got[0] != moved {
		t.Errorf("the Spec Branch's first commit is on %v, want the moved base %s", got, moved)
	}
	for i, orig := range append(firstCommits, secondCommits...) {
		if got, want := h.Subject(commits[i]), h.Subject(orig); got != want {
			t.Errorf("rebased commit %d is %q, want %q", i, got, want)
		}
	}
	r.WantOutput(h.SpecID() + ": rebased " + spec + " onto main at " + tip[:12])
	if m := h.Ticket(second).Moves; m[len(m)-1].Commit != h.Tip("main") {
		t.Errorf("%s landed at %q, want the Merge spec commit", second, m[len(m)-1].Commit)
	}
}

func TestUnderRebaseAMixedSpecBranchKeepsItsEarlierMergesAsMergesWhenRebasedOntoAMovedBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.MakeRequirement()
	release := holdTicketsStage(t, h)
	first, _ := accepted(h, "First", "one.txt", "1\n")
	firstHead := h.Tip(first)
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
	subs, merges := specLine(h, h, moved)
	if got, want := strings.Join(subs, ", "), "Merge "+first+": First, Change two.txt"; got != want {
		t.Fatalf("the Spec Branch's line on the moved base is %q, want %q", got, want)
	}
	if len(merges) != 1 {
		t.Fatalf("the Spec Branch's line holds merges %q, want the earlier Merge commit kept as one", merges)
	}
	line, _ := inRange(h, h.Tip(spec), moved)
	if got := strings.Join(h.Parents(line[0]), " "); got != moved+" "+firstHead {
		t.Errorf("the remade merge has parents %q, want the moved base %s and %s's head %s", got, moved, first, firstHead)
	}
	if got := strings.Join(h.Parents(h.Tip("main")), " "); got != moved+" "+h.Tip(spec) {
		t.Errorf("main's tip has parents %q, want the moved base then the rebased Spec Branch", got)
	}
	if got := mergeSpecs(h, moved); len(got) != 1 {
		t.Errorf("main holds %d Merge spec commits, want exactly one", len(got))
	}
}

func TestUnderRebaseASpecBranchConflictingInOneRepositoryRebasesAndLandsNothingAnywhere(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
	apiRepo, id := specInTwo(h)
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
	homeTip, apiTip := h.Tip(spec), apiRepo.Tip(spec)
	// The Home Repository's base moves on cleanly; api's conflicts.
	homeBase := h.CommitOn("main", "base.txt", "base\n")
	apiBase := apiRepo.CommitOn("main", "endpoint.go", "package other\n")

	release()
	r.Poll()

	h.WantState(id, domain.Accepted)
	if h.Tip("main") != homeBase || apiRepo.Tip("main") != apiBase {
		t.Fatal("a base moved: nothing lands while any Repository's Spec Branch conflicts")
	}
	if h.Tip(spec) != homeTip || apiRepo.Tip(spec) != apiTip {
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
	h := factorytest.New(t, factorytest.RealGit())
	remote := h.AddRemote("origin")
	on := true
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingMerge, Push: true, Remote: "origin", Rebase: &on})
	h.Publish(remote, "main", "main")
	h.MakeRequirement()
	a, _ := accepted(h, "First", "one.txt", "1\n")
	b, _ := accepted(h, "Second", "two.txt", "2\n")
	release := holdTicketsStage(t, h)
	r := h.Start()
	r.WantOutput(b + ": replayed onto spec/" + h.SpecID())

	// The base moves on, pushed, so the Spec Branch is rebased at landing.
	moved := h.CommitOn("main", "base.txt", "base\n")
	h.Publish(remote, "main", "main")
	remote.RefusePushes()
	release()
	r.Poll()
	spec := "spec/" + h.SpecID()
	r.WantOutput(h.SpecID() + ": rebased " + spec + " onto main at ")
	r.WantOutput(a + ": can't land yet")
	specTip := h.Tip(spec)
	if !descendsFrom(h, specTip, moved) {
		t.Fatalf("the Spec Branch's tip %s isn't on the moved base %s", specTip, moved)
	}

	// Meanwhile the remote's base moves on too.
	h.Git("branch", "theirs", moved)
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	remote.AcceptPushes()
	h.Publish(remote, theirs, "main")
	r.Poll()
	r.WantOutput("main: moved 1 unpushed commit onto origin/main")
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
	if got := strings.Join(h.Parents(h.Tip("main")), " "); got != theirs+" "+specTip {
		t.Errorf("main's tip has parents %q, want the remote's %s then the rebased Spec Branch's %s", got, theirs, specTip)
	}
	if got := remote.Tip("main"); got != h.Tip("main") {
		t.Errorf("the remote's main is at %q, want it pushed level with the local base", got)
	}
}
