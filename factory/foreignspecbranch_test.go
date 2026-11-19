package factory_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
)

// A foreign remote branch (code-central-1096): under Pull Request, the
// Factory doesn't push a Spec Branch over a branch of that name on the
// Push remote that it didn't push, forced or not. It asks the human to
// delete or rename that branch, and pushes once it's gone.

// handMadeOn pushes a hand-made commit to remote as branch, a branch the
// Factory didn't push, and returns the commit.
func handMadeOn(h *factorytest.Harness, remote *factorytest.Remote, branch string) string {
	hand := h.CommitOn("handmade", "hand.txt", "hand\n")
	h.Git("push", "-q", remote.Path, "handmade:refs/heads/"+branch)
	return hand
}

// wantForeignBranchCommentOnce checks each of ids has the can't-push
// comment naming branch on origin exactly once.
func wantForeignBranchCommentOnce(t *testing.T, h *factorytest.Harness, branch string, ids ...string) {
	t.Helper()
	for _, id := range ids {
		var n int
		for _, c := range h.Ticket(id).Comments {
			if strings.Contains(c.Body, "can't push") && strings.Contains(c.Body, "origin already has a branch named "+branch) {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%s has %d can't-push comments naming %s on origin, want 1: %+v", id, n, branch, h.Ticket(id).Comments)
		}
	}
}

// wantRemoteAt fails when remote's branch isn't at commit.
func wantRemoteAt(t *testing.T, remote *factorytest.Remote, branch, commit string) {
	t.Helper()
	if got := remote.Git("rev-parse", branch); got != commit {
		t.Errorf("the remote's %s is at %q, want the hand-made %s left untouched", branch, got, commit)
	}
}

func TestUnderPullRequestATaggedSpecBranchIsntPushedOverABranchOfThatNameTheFactoryDidntPush(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	taggedRequirement(t, h)
	hand := handMadeOn(h, remote, taggedBranch)
	a, _ := accepted(h, "First", "one.txt", "1\n")
	b, _ := accepted(h, "Second", "two.txt", "2\n")

	r := h.Start()
	r.Poll()
	wantRemoteAt(t, remote, taggedBranch, hand)
	wantForeignBranchCommentOnce(t, h, taggedBranch, a, b)
	if tk := h.Ticket(a); !commented(tk, "Delete or rename it on origin") {
		t.Errorf("%s's comments %+v don't ask the human to delete or rename the branch", a, tk.Comments)
	}
	if strings.Contains(r.Stdout(), ": pushed "+taggedBranch) {
		t.Errorf("the Factory pushed %s over the hand-made branch:\n%s", taggedBranch, r.Stdout())
	}

	// Once the human deletes it, a later poll pushes the Spec Branch.
	remote.Git("branch", "-D", taggedBranch)
	r.Poll()
	r.WantOutput(h.SpecID() + ": pushed " + taggedBranch + " to origin")
	tip := h.Git("rev-parse", taggedBranch)
	if got := remote.Git("rev-parse", taggedBranch); got != tip {
		t.Errorf("the remote's %s is at %q, want the Spec Branch %s", taggedBranch, got, tip)
	}
	wantPushed(t, h, taggedBranch, tip)
}

func TestUnderPullRequestWithRebaseASpecBranchIsntForcePushedOverABranchOfThatNameTheFactoryDidntPush(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	rebasePullRequestOn(h, "origin", true)
	h.MakeRequirement()
	branch := core.SpecBranch(h.SpecID(), nil)
	hand := handMadeOn(h, remote, branch)
	a, _ := accepted(h, "First", "one.txt", "1\n")

	r := h.Start()
	r.Poll()
	wantRemoteAt(t, remote, branch, hand)
	wantForeignBranchCommentOnce(t, h, branch, a)

	remote.Git("branch", "-D", branch)
	r.Poll()
	r.WantOutput(h.SpecID() + ": pushed " + branch + " to origin")
	tip := h.Git("rev-parse", branch)
	if got := remote.Git("rev-parse", branch); got != tip {
		t.Errorf("the remote's %s is at %q, want the Spec Branch %s", branch, got, tip)
	}
}

func TestUnderPullRequestWithRebaseAFactoryPushedSpecBranchIsStillForcePushed(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	rebasePullRequestOn(h, "origin", true)
	h.MakeRequirement()
	branch := core.SpecBranch(h.SpecID(), nil)
	a, _ := accepted(h, "First", "one.txt", "1\n")

	r := h.Start()
	r.WantOutput(h.SpecID() + ": pushed " + branch + " to origin")
	first := h.Git("rev-parse", branch)

	moveRemoteBase(h, remote, "base.txt", "base\n")
	accepted(h, "Second", "two.txt", "2\n")
	r.Poll()
	tip := h.Git("rev-parse", branch)
	if tip == first {
		t.Fatalf("%s wasn't rebased", branch)
	}
	if got := remote.Git("rev-parse", branch); got != tip {
		t.Errorf("the remote's %s is at %q, want the rebased Spec Branch %s over the Factory's own push %s", branch, got, tip, first)
	}
	if commented(h.Ticket(a), "already has a branch named") {
		t.Errorf("%s was told the Factory's own Spec Branch is foreign: %+v", a, h.Ticket(a).Comments)
	}
}
