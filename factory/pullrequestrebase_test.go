package factory_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/factory/factorytest"
)

// Pull Request under Rebase (Spec code-central-1063, section "Pull
// Request"): just before it pushes a Ticket's branch, or with Land per
// Spec a Spec Branch, that doesn't descend from the fetched base's tip,
// the Factory rebases it onto that tip, and it pushes a branch the remote
// holds an older version of with `--force-with-lease`. It rebases only
// when it is about to push, never on a base move alone, and never
// force-pushes a base.

// rebasePullRequestOn sets the Pull Request Landing Method with the Push
// remote, Rebase on and Land per Spec as landPerSpec says.
func rebasePullRequestOn(h *factorytest.Harness, remote string, landPerSpec bool) {
	// The harness's remote is a path; Pull Request needs a Code Host's URL.
	if _, err := h.Server.Client.ReportPushRemote(context.Background(), h.Project, api.ReportPushRemoteInput{URL: "https://github.com/acme/app.git", Base: "main"}); err != nil {
		panic(err)
	}
	on := true
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingPullRequest, Remote: remote, Rebase: &on, LandPerSpec: &landPerSpec})
}

// moveRemoteBase moves the remote's main on, as a pull request merged on
// the Code Host does, by a commit changing file on top of the local
// main, which the Factory's next fetch brings in. It returns the commit.
func moveRemoteBase(h *factorytest.Harness, remote *factorytest.Remote, file, content string) string {
	moved := h.CommitOn("upstream", file, content)
	h.Publish(remote, moved, "main")
	return moved
}

// stalePush puts branch on the remote at a commit of its own, cut from
// main, as a push of an earlier finish of its Ticket would have left it,
// and returns that commit.
func stalePush(h *factorytest.Harness, remote *factorytest.Remote, branch string) string {
	stale := h.CommitOn("stale-"+branch, "stale.txt", "an earlier finish\n")
	h.Publish(remote, stale, branch)
	return stale
}

func TestUnderPullRequestWithRebaseATicketIsRebasedOntoTheFetchedBaseAndForcePushedWithLease(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	rebasePullRequestOn(h, "origin", false)
	// With CI Tracking, the pushed commit is the one CI results count on.
	trackCI(t, h)
	id, commits := acceptedTwice(h, "Add a feature", "feature")
	stale := stalePush(h, remote, id)
	moved := moveRemoteBase(h, remote, "base.txt", "base\n")

	r := h.Start()
	r.WantOutput(id + ": replayed onto main at ")
	r.WantOutput(id + ": pushed " + id + " to origin")

	tip := h.Tip(id)
	if got := remote.Tip(id); got != tip {
		t.Fatalf("the remote's %s is at %s, want the rebased branch %s over the earlier push %s", id, got, tip, stale)
	}
	line, merges := inRange(h, tip, moved)
	if len(line) != 2 || len(merges) > 0 {
		t.Fatalf("the branch adds %d commits with merges %v to the fetched base, want the 2 replayed and no merge", len(line), merges)
	}
	if got := h.Parents(line[0]); len(got) != 1 || got[0] != moved {
		t.Errorf("the first replayed commit's parents are %v, want the fetched base's tip %s", got, moved)
	}
	for i, orig := range commits {
		if got, want := h.Subject(line[i]), h.Subject(orig); got != want {
			t.Errorf("replayed commit %d is %q, want %q", i, got, want)
		}
	}
	wantPushed(t, h, id, tip)
	tk := h.Ticket(id)
	if tk.State != domain.Accepted || tk.PushedCommit == nil || tk.PushedCommit.Commit != tip {
		t.Errorf("%s is %s with pushed commit %+v, want it Accepted, pushed at the force-pushed tip %s", id, tk.State, tk.PushedCommit, tip)
	}
	if got := remote.Tip("main"); got != moved {
		t.Errorf("the remote's main is at %s, want it left at %s", got, moved)
	}
}

func TestUnderPullRequestWithRebaseAPushedTicketIsntRebasedWhenTheBaseMoves(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	rebasePullRequestOn(h, "origin", false)
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	wantPushed(t, h, id, commit)
	moved := moveRemoteBase(h, remote, "base.txt", "base\n")
	r.Poll()

	if got := h.Tip("main"); got != moved {
		t.Fatalf("main is at %s, want it fast-forwarded to the remote's %s", got, moved)
	}
	if got := remote.Tip(id); got != commit {
		t.Errorf("the remote's %s is at %s, want it left at the push %s", id, got, commit)
	}
	if got := h.Tip(id); got != commit {
		t.Errorf("%s's branch is at %s, want it left at %s", id, got, commit)
	}
	if strings.Contains(r.Stdout(), id+": replayed onto") {
		t.Error("a branch already pushed was rebased on a base move")
	}
}

func TestUnderPullRequestWithRebaseATicketConflictingWithTheFetchedBaseIsSentBackToRebase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	rebasePullRequestOn(h, "origin", false)
	id, _ := accepted(h, "Add a feature", "feature.txt", "ticket side\n")
	moveRemoteBase(h, remote, "feature.txt", "base side\n")

	h.Script(id, factorytest.Wait())
	h.Start()
	tk := h.Ticket(id)
	if !commented(tk, "replaying", "onto main", "- feature.txt", "Rebase the branch onto main, resolve the conflicts, and finish again.") || commented(tk, "Merge main into the branch") {
		t.Errorf("want a send-back note asking to rebase onto main, got %+v", tk.Comments)
	}
	c := callFor(t, h, id)
	requireConflictClaim(t, c, "feature.txt")
	if !strings.Contains(c.Prompt, "Rebase the branch onto main and resolve the conflicts") {
		t.Errorf("first prompt %q, want it to say to rebase onto main", c.Prompt)
	}
	if remote.Exists(id) {
		t.Error("the conflicting branch was pushed")
	}
}

func TestUnderPullRequestWithRebaseASpecBranchIsRebasedOntoTheFetchedBaseAndForcePushedWithLease(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	rebasePullRequestOn(h, "origin", true)
	trackCI(t, h)
	h.MakeRequirement()
	a, _ := accepted(h, "First", "one.txt", "1\n")
	branch := "spec/" + h.SpecID()

	r := h.Start()
	r.WantOutput(h.SpecID() + ": pushed " + branch + " to origin")
	first := h.Tip(branch)
	wantPushed(t, h, branch, first)

	// The base moves on the Code Host, and a Ticket added to the Spec is
	// accepted onto the pushed Spec Branch.
	moved := moveRemoteBase(h, remote, "base.txt", "base\n")
	b, _ := accepted(h, "Second", "two.txt", "2\n")
	r.Poll()
	r.WantOutput(h.SpecID() + ": rebased " + branch + " onto main at ")

	tip := h.Tip(branch)
	if got := remote.Tip(branch); got != tip {
		t.Fatalf("the remote's %s is at %s, want the rebased Spec Branch %s over the earlier push %s", branch, got, tip, first)
	}
	subs, merges := specLine(h, h, moved)
	if strings.Join(subs, ",") != "Change one.txt,Change two.txt" || len(merges) > 0 {
		t.Errorf("%s holds %v with merges %v on the fetched base, want a's and b's commits and no merge", branch, subs, merges)
	}
	if !descendsFrom(h, tip, moved) {
		t.Errorf("%s at %s doesn't descend from the fetched base's tip %s", branch, tip, moved)
	}
	wantPushed(t, h, branch, tip)
	for _, id := range []string{a, b} {
		if tk := h.Ticket(id); tk.PushedCommit == nil || tk.PushedCommit.Commit != tip {
			t.Errorf("%s's pushed commit is %+v, want the force-pushed tip %s", id, tk.PushedCommit, tip)
		}
	}
	if got := remote.Tip("main"); got != moved {
		t.Errorf("the remote's main is at %s, want it left at %s", got, moved)
	}
}

func TestUnderPullRequestWithRebaseAPushedSpecBranchIsntRebasedWhenTheBaseMoves(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	rebasePullRequestOn(h, "origin", true)
	h.MakeRequirement()
	accepted(h, "First", "one.txt", "1\n")
	branch := "spec/" + h.SpecID()

	r := h.Start()
	r.WantOutput(h.SpecID() + ": pushed " + branch + " to origin")
	first := h.Tip(branch)
	moveRemoteBase(h, remote, "base.txt", "base\n")
	r.Poll()

	if got := h.Tip(branch); got != first {
		t.Errorf("%s moved to %s on a base move, want it left at %s", branch, got, first)
	}
	if got := remote.Tip(branch); got != first {
		t.Errorf("the remote's %s is at %s, want it left at %s", branch, got, first)
	}
}

func TestUnderPullRequestWithRebaseASpecBranchConflictingWithTheFetchedBaseOpensTheRebaseTicket(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	rebasePullRequestOn(h, "origin", true)
	h.MakeRequirement()
	accepted(h, "First", "one.txt", "1\n")
	branch := "spec/" + h.SpecID()

	r := h.Start()
	r.WantOutput(h.SpecID() + ": pushed " + branch + " to origin")
	first := h.Tip(branch)
	moveRemoteBase(h, remote, "two.txt", "base side\n")
	accepted(h, "Second", "two.txt", "spec side\n")
	r.Poll()

	r.WantOutput(h.SpecID() + ": " + branch + " conflicts with main in two.txt: opened ")
	r.WantOutput(" to rebase it onto the base")
	if got := remote.Tip(branch); got != first {
		t.Errorf("the remote's %s is at %s, want it left at %s while it conflicts", branch, got, first)
	}
}

func TestUnderPullRequestWithRebaseATicketConflictingInOneRepositoryIsRebasedAndPushedInNeither(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
	home := h.AddRemote("origin")
	apiRepo := h.AddRepository("api")
	rebasePullRequestOn(h, "origin", false)
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)
	// Both remote bases move on: the Home Repository's cleanly, api's in
	// conflict.
	moveRemoteBase(h, home, "base.txt", "base\n")
	apiUpstream := apiRepo.CommitOn("upstream", "endpoint.go", "package other\n")
	apiRepo.Publish(apiRepo.Remote("origin"), apiUpstream, "main")
	homeBranch, apiBranch := h.Tip(id), apiRepo.Tip(id)
	h.Script(id, factorytest.Wait())

	h.Accept(id)
	r.Poll()

	tk := h.Ticket(id)
	if !movedTo(tk, domain.Accepted, domain.Ready) {
		t.Fatalf("%s wasn't sent back: %+v", id, tk.Moves)
	}
	if h.Tip(id) != homeBranch || apiRepo.Tip(id) != apiBranch {
		t.Error("a branch moved: a Ticket conflicting in any Repository is rebased in none")
	}
	if home.Exists(id) || apiRepo.Remote("origin").Exists(id) {
		t.Error("a branch was pushed: a Ticket conflicting in any Repository is pushed in none")
	}
	if !commented(tk, "- api: replaying", "  - endpoint.go", "In each, rebase the branch onto its base") || commented(tk, "- "+h.Project+":") {
		t.Fatalf("comments %+v, want the send-back note to name api and its file alone, and say to rebase", tk.Comments)
	}
}
