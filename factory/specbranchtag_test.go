package factory_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/factory/factorytest"
)

// A Spec's branch Tag names its Spec Branch (Spec code-central-1089,
// section "Factory"): under Land per Spec the Factory makes, merges into,
// rebases, pushes and lands the Spec Branch under that name, the same in
// every Repository. With Land per Spec off, under Branch, or on a
// Long-lived Spec it changes nothing.

// taggedBranch is the branch Tag these tests give the Spec.
const taggedBranch = "feat/4242"

// tagSpec gives the harness's Spec the branch Tag taggedBranch.
func tagSpec(t *testing.T, h *factorytest.Harness) {
	t.Helper()
	if _, err := h.Human().SetTags(context.Background(), h.Project, api.SetTagsInput{Target: api.Ref(h.SpecID()), Tags: map[string]string{core.BranchTag: taggedBranch}}); err != nil {
		t.Fatal(err)
	}
}

// taggedRequirement is a Requirement Spec with the branch Tag
// taggedBranch.
func taggedRequirement(t *testing.T, h *factorytest.Harness) {
	t.Helper()
	h.MakeRequirement()
	tagSpec(t, h)
}

// wantNoDefaultSpecBranch fails when the repository at dir has a branch
// under spec/, the name a Spec with no branch Tag would have.
func wantNoDefaultSpecBranch(t *testing.T, h *factorytest.Harness, dir string) {
	t.Helper()
	if got := h.GitIn(dir, "branch", "--list", "spec/*"); got != "" {
		t.Errorf("a spec/ branch was made in %s: %q", dir, got)
	}
}

// wantNoBranch fails when the repository at dir has branch.
func wantNoBranch(t *testing.T, h *factorytest.Harness, dir, branch string) {
	t.Helper()
	if got := h.GitIn(dir, "branch", "--list", branch); got != "" {
		t.Errorf("%s was made in %s: %q", branch, dir, got)
	}
}

func TestATaggedSpecsFirstClaimMakesTheSpecBranchUnderTheTagsNameAndCutsTheTicketFromIt(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	taggedRequirement(t, h)
	id := h.ReadyTicket("Add a feature")
	h.Script(id, factorytest.Wait())
	base := h.Git("rev-parse", "main")

	h.Start()

	if got := h.Git("rev-parse", taggedBranch); got != base {
		t.Errorf("the Spec Branch %s is at %s, want the base's tip %s", taggedBranch, got, base)
	}
	if got := h.GitIn(h.Worktree(id), "rev-parse", "HEAD"); got != base {
		t.Errorf("the Ticket's branch is at %s, want the Spec Branch's tip %s", got, base)
	}
	wantNoDefaultSpecBranch(t, h, h.Repo)
}

func TestUnderMergeATaggedSpecLandsThroughABranchOfTheTagsName(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	taggedRequirement(t, h)
	a, _ := accepted(h, "First", "one.txt", "1\n")
	b, _ := accepted(h, "Second", "two.txt", "2\n")

	r := h.Start()
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)

	r.WantOutput(a + ": merged into " + taggedBranch + " at ")
	r.WantOutput(h.SpecID() + ": merged " + taggedBranch + " into main at ")
	if got, want := h.Git("log", "-1", "--format=%P", "main"), h.Git("rev-parse", taggedBranch); !strings.HasSuffix(got, " "+want) {
		t.Errorf("main's tip has parents %q, want the Spec Branch %s's tip %s second", got, taggedBranch, want)
	}
	if got := h.Git("log", "-1", "--format=%s", "main"); got != "Merge spec "+h.SpecID()+": Work" {
		t.Errorf("merge subject %q", got)
	}
	wantNoDefaultSpecBranch(t, h, h.Repo)
}

func TestATaggedSpecBranchConflictOpensTheSpecMergeTicketNamingTheTagsBranch(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	taggedRequirement(t, h)
	a, _ := accepted(h, "First", "README", "spec side\n")
	release := holdTicketsStage(t, h)
	r := h.Start()
	r.WantOutput(a + ": merged into " + taggedBranch)

	if err := os.WriteFile(filepath.Join(h.Repo, "README"), []byte("base side\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.Git("add", "README")
	h.Git("commit", "-q", "-m", "base changes README")
	release()
	r.Poll()
	r.WantOutput(h.SpecID() + ": " + taggedBranch + " conflicts with main in README: opened ")
	r.Poll()

	// The Spec merge Ticket, its branch not cut yet, is checked at the
	// Tag's branch, so it starts as a Conflict Claim.
	rows, err := h.Server.Client.FindTickets(context.Background(), h.Project, api.FindTicketsInput{Title: api.SpecMergeTitle(h.SpecID())})
	if err != nil || len(rows) != 1 {
		t.Fatalf("Spec merge Tickets %+v (%v), want one", rows, err)
	}
	requireConflictClaim(t, callFor(t, h, rows[0].ID), "README")
	wantNoDefaultSpecBranch(t, h, h.Repo)
}

func TestATaggedSpecsTicketCutFromItsSpecBranchDoesntCountARepositoryItLeftAloneChanged(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	apiRepo := h.AddRepository("api")
	tagSpec(t, h)
	first := inTwo(h, "Add an endpoint")
	holdTicketsStage(t, h)
	r := h.Start()
	h.WantState(first, domain.InReview)
	h.Accept(first)
	r.Poll()
	r.WantOutput(first + ": merged into " + taggedBranch)

	// The second Ticket's api branch is cut at the Spec Branch's tip, which
	// holds the first's work there, and it commits in the Home alone.
	second := h.ReadyTicket("Call it")
	h.Script(second, factorytest.WriteFile("call.go", "package acme\n"), factorytest.Git("add", "call.go"), factorytest.Git("commit", "-q", "-m", "Call it"), factorytest.Wait())
	r.PollUntil("the Home's work to be reported live", func() bool { return len(h.Ticket(second).Repositories) > 0 })
	home := h.GitIn(h.Repo, "rev-parse", second)
	if got := changed(h.Ticket(second)); len(got) != 1 || got[0] != "acme@"+home {
		t.Errorf("Changed Repositories %v, want the Home's alone at %s: api's branch holds nothing beyond %s there", got, home, taggedBranch)
	}
	if got := h.GitIn(apiRepo.Path, "rev-parse", second); got != h.GitIn(apiRepo.Path, "rev-parse", taggedBranch) {
		t.Errorf("api's branch is at %s, want %s's tip", got, taggedBranch)
	}
}

func TestUnderMergeAndPushAnUnpushedTaggedSpecBranchMergeIsRebuiltWhenTheBaseMoves(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	h.Git("push", "-q", remote.Path, "main:refs/heads/main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	taggedRequirement(t, h)
	a, _ := accepted(h, "First", "one.txt", "1\n")
	b, _ := accepted(h, "Second", "two.txt", "2\n")
	remote.RefusePushes()
	r := h.Start()
	r.WantOutput(h.SpecID() + ": merged " + taggedBranch + " into main at ")
	r.WantOutput(a + ": can't land yet")
	remote.AcceptPushes()
	specTip := h.Git("rev-parse", taggedBranch)
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")

	r.Poll()
	r.WantOutput("main: moved 1 unpushed commit onto origin/main")
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
	if got := h.Git("log", "-1", "--format=%P", "main"); got != theirs+" "+specTip {
		t.Errorf("main's tip has parents %q, want the remote's %s then the Spec Branch's %s", got, theirs, specTip)
	}
}

func TestUnderPullRequestATaggedSpecBranchIsPushedAndItsPullRequestOpenedFromTheTagsName(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	taggedRequirement(t, h)
	a, _ := accepted(h, "First", "one.txt", "1\n")
	b, _ := accepted(h, "Second", "two.txt", "2\n")

	r := h.Start()
	r.WantOutput(h.SpecID() + ": pushed " + taggedBranch + " to origin")
	tip := h.Git("rev-parse", taggedBranch)
	if got := remote.Git("rev-parse", taggedBranch); got != tip {
		t.Errorf("the remote's %s is at %q, want the local one's tip %s", taggedBranch, got, tip)
	}
	wantPushed(t, h, taggedBranch, tip)
	wantNoDefaultSpecBranch(t, h, h.Repo)
	wantNoDefaultSpecBranch(t, h, remote.Path)

	// The pull request from the Tag's branch merges on the host.
	squash := h.CommitOn("squash", "one.txt", "1\n")
	h.Git("push", "-q", remote.Path, "squash:refs/heads/main")
	reportPullRequest(t, h, taggedBranch, squash)
	r.Poll()
	for _, id := range []string{a, b} {
		tk := h.WantState(id, domain.Done)
		if m := tk.Moves[len(tk.Moves)-1]; m.Commit != squash {
			t.Errorf("%s landed at %q, want the merge commit %s", id, m.Commit, squash)
		}
	}
}

func TestUnderPullRequestWithRebaseATaggedSpecBranchIsForcePushedWithLeaseUnderTheTagsName(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	rebasePullRequestOn(h, "origin", true)
	trackCI(t, h)
	taggedRequirement(t, h)
	a, _ := accepted(h, "First", "one.txt", "1\n")

	r := h.Start()
	r.WantOutput(h.SpecID() + ": pushed " + taggedBranch + " to origin")
	first := h.Git("rev-parse", taggedBranch)
	wantPushed(t, h, taggedBranch, first)

	moved := moveRemoteBase(h, remote, "base.txt", "base\n")
	b, _ := accepted(h, "Second", "two.txt", "2\n")
	r.Poll()
	r.WantOutput(h.SpecID() + ": rebased " + taggedBranch + " onto main at ")

	tip := h.Git("rev-parse", taggedBranch)
	if got := remote.Git("rev-parse", taggedBranch); got != tip {
		t.Fatalf("the remote's %s is at %s, want the rebased Spec Branch %s over the earlier push %s", taggedBranch, got, tip, first)
	}
	if !descends(h, h.Repo, tip, moved) {
		t.Errorf("%s at %s doesn't descend from the fetched base's tip %s", taggedBranch, tip, moved)
	}
	for _, id := range []string{a, b} {
		if tk := h.Ticket(id); tk.PushedCommit == nil || tk.PushedCommit.Commit != tip {
			t.Errorf("%s's pushed commit is %+v, want the force-pushed tip %s", id, tk.PushedCommit, tip)
		}
	}
	wantNoDefaultSpecBranch(t, h, remote.Path)
}

func TestATaggedTwoRepositorySpecLandsThroughABranchOfTheSameNameInEach(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	h.MakeRequirement()
	apiRepo := h.AddRepository("api")
	tagSpec(t, h)
	id := inTwo(h, "Add an endpoint")
	mergeAndPush(h, "origin")
	r := h.Start()
	h.WantState(id, domain.InReview)

	h.Accept(id)
	r.Poll()

	h.WantState(id, domain.Done)
	r.WantOutput(h.SpecID() + ": merged " + taggedBranch + " into main at ")
	r.WantOutput(" in Repository api\n")
	for _, dir := range []string{h.Repo, apiRepo.Path} {
		tip := h.GitIn(dir, "rev-parse", taggedBranch)
		if got := h.GitIn(dir, "log", "-1", "--format=%P", "main"); !strings.HasSuffix(got, " "+tip) {
			t.Errorf("main in %s has parents %q, want %s's tip %s second", dir, got, taggedBranch, tip)
		}
		wantNoDefaultSpecBranch(t, h, dir)
	}
}

func TestUnderPullRequestATaggedTwoRepositorySpecBranchIsPushedUnderTheTagsNameInEach(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	home := h.AddRemote("origin")
	h.MakeRequirement()
	apiRepo := h.AddRepository("api")
	tagSpec(t, h)
	id := inTwo(h, "Add an endpoint")
	pullRequestOn(h, "origin")
	r := h.Start()
	h.WantState(id, domain.InReview)

	h.Accept(id)
	r.Poll()

	r.WantOutput(h.SpecID() + ": pushed " + taggedBranch + " to origin in Repository api")
	apiTip, homeTip := wantPushedIn(t, h, "api", taggedBranch), wantPushedIn(t, h, h.Project, taggedBranch)
	if got := apiRepo.Remote("origin").Git("rev-parse", taggedBranch); got != apiTip || apiTip != h.GitIn(apiRepo.Path, "rev-parse", taggedBranch) {
		t.Errorf("api's remote %s is at %q, want its local Spec Branch %s", taggedBranch, got, apiTip)
	}
	if got := home.Git("rev-parse", taggedBranch); got != homeTip || homeTip != h.Git("rev-parse", taggedBranch) {
		t.Errorf("the Home Repository's remote %s is at %q, want its local Spec Branch %s", taggedBranch, got, homeTip)
	}
}

func TestWithLandPerSpecOffTheBranchTagChangesNothing(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	off := false
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingMerge, LandPerSpec: &off})
	taggedRequirement(t, h)
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")

	h.Start()
	h.WantState(id, domain.Done)
	if got := h.Git("log", "-1", "--format=%s", "main"); !strings.HasPrefix(got, "Merge "+id) {
		t.Errorf("main's tip is %q, want a plain merge of %s", got, id)
	}
	wantNoBranch(t, h, h.Repo, taggedBranch)
}

func TestUnderTheBranchMethodTheBranchTagChangesNothing(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingBranch})
	taggedRequirement(t, h)
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	h.Start()
	if h.Ticket(id).State != domain.Accepted {
		t.Errorf("want %s left Accepted", id)
	}
	wantNoBranch(t, h, h.Repo, taggedBranch)
}

func TestOnALongLivedSpecTheBranchTagChangesNothing(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.ReadyTicket("Make the shared Spec")
	tagSpec(t, h)
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")

	h.Start()
	h.WantState(id, domain.Done)
	if got := h.Git("log", "-1", "--format=%s", "main"); !strings.HasPrefix(got, "Merge "+id) {
		t.Errorf("main's tip is %q, want a plain merge of %s", got, id)
	}
	wantNoBranch(t, h, h.Repo, taggedBranch)
}

func TestATaggedSpecBranchThatCantBeCreatedFailsTheClaimNamingTheTagsBranch(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	taggedRequirement(t, h)
	// A branch named feat is in the way of feat/4242.
	h.Git("branch", "feat")
	id := h.ReadyTicket("Add a feature")
	h.Script(id, factorytest.Wait())

	h.Start()

	tk := h.Ticket(id)
	if len(tk.FailureReports) == 0 {
		t.Fatalf("%s is %s with no Failure Report", id, tk.State)
	}
	if fr := tk.FailureReports[0]; !strings.Contains(fr.WhatBroke, "Spec Branch "+taggedBranch) || !strings.Contains(fr.WhatBroke, `named "feat" may be`) || strings.Contains(fr.WhatBroke, `"spec"`) {
		t.Errorf("the Failure Report says %q, want it to name %s, and feat as in its way, not a branch called spec", fr.WhatBroke, taggedBranch)
	}
}
