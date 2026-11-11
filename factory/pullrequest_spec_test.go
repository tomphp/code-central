package factory_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// Land per Spec under Pull Request (code-central-395): the Spec Branch is
// pushed once the Spec is ready, with one pull request, and its Tickets
// land together at that pull request's merge commit.

func TestUnderPullRequestAReadySpecBranchIsPushedAndReportedForEveryTicket(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	a, b := twoAccepted(t, h)
	branch := "spec/" + h.SpecID()

	r := h.Start()
	r.WaitForOutput(h.SpecID() + ": pushed " + branch + " to origin")
	tip := h.Git("rev-parse", branch)
	if got := remote.Git("rev-parse", branch); got != tip {
		t.Errorf("the remote's %s is at %q, want %s", branch, got, tip)
	}
	if got := remote.Git("branch", "--list", "main"); strings.TrimSpace(got) != "" {
		t.Errorf("the remote has main %q: only the Spec Branch is pushed", got)
	}
	waitPushed(t, h, branch, tip)
	for _, id := range []string{a, b} {
		if tk := h.Ticket(id); tk.State != store.Accepted {
			t.Errorf("%s is %s, want it waiting Accepted", id, tk.State)
		}
	}
	w, err := h.Server.Client.GetPullRequestWatch(context.Background(), h.Project)
	if err != nil || len(w.Work) != 1 || w.Work[0].Title != "Work" || len(w.Work[0].Tickets) != 2 {
		t.Errorf("watch %+v (%v), want one piece of work for both Tickets, titled with the Spec's", w.Work, err)
	}
}

func TestUnderPullRequestTheSpecsTicketsLandAtItsMergeCommit(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	a, b := twoAccepted(t, h)
	cancelDrafts(t, h)
	branch := "spec/" + h.SpecID()

	r := h.Start()
	waitPushed(t, h, branch, "")
	reportPullRequest(t, h, branch, "")
	r.Poll()
	if h.Ticket(a).State != store.Accepted || h.Ticket(b).State != store.Accepted {
		t.Fatal("a Ticket landed before its pull request merged")
	}

	// A squash merge on the remote.
	squash := h.CommitOn("squash", "one.txt", "1\n")
	h.Git("push", "-q", remote.Path, "squash:refs/heads/main")
	reportPullRequest(t, h, branch, squash)
	r.Poll()

	for _, id := range []string{a, b} {
		tk := h.WaitForState(id, store.Done)
		if m := tk.Moves[len(tk.Moves)-1]; m.Commit != squash {
			t.Errorf("%s landed at %q, want the merge commit %s", id, m.Commit, squash)
		}
	}
	r.WaitForOutput(b + ": landed on main at ")
	r.Poll()
	r.WaitForOutput(h.SpecID() + ": deleted " + branch)
}

func TestUnderPullRequestASpecBranchThatConflictsWithTheBaseOpensTheConflictTicketAndIsPushedOnceFixed(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	h.MakeRequirement()
	a, _ := accepted(h, "First", "README", "spec side\n")
	release := holdTicketsStage(t, h)
	fix := "acme-5"
	h.Script(fix, factorytest.Git("merge", "-q", "-X", "theirs", "main"), factorytest.CLI("finish", "{{ticket}}", "--claim", "{{claim}}", "--summary", "Merged the base.", "--branch", fix))
	branch := "spec/" + h.SpecID()
	r := h.Start()
	r.WaitForOutput(a + ": merged into " + branch)

	if err := os.WriteFile(filepath.Join(h.Repo, "README"), []byte("base side\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.Git("add", "README")
	h.Git("commit", "-q", "-m", "base changes README")
	h.Git("push", "-q", remote.Path, "main:refs/heads/main")
	release()
	r.Poll()
	r.WaitForOutput(h.SpecID() + ": " + branch + " conflicts with main in README: opened " + fix)
	if out := remote.Git("branch", "--list", branch); strings.TrimSpace(out) != "" {
		t.Fatalf("the conflicting Spec Branch was pushed: %q", out)
	}
	h.WaitForState(fix, store.InReview)

	h.Accept(fix)
	r.Poll()
	h.WaitFor("the fixed Spec Branch is pushed", func() bool {
		return strings.TrimSpace(remote.Git("branch", "--list", branch)) != ""
	})
	tip := h.Git("rev-parse", branch)
	waitPushed(t, h, branch, tip)
	if got := remote.Git("rev-parse", branch); got != tip {
		t.Errorf("the remote's %s is at %s, want %s", branch, got, tip)
	}
}

func TestUnderPullRequestATicketOutsideLandPerSpecStillGetsItsOwnPullRequest(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	off := false
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingBranch, LandPerSpec: &off})
	pullRequestOn(h, "origin")
	h.MakeRequirement()
	id, commit := accepted(h, "Plain", "plain.txt", "plain\n")
	h.Start()
	waitPushed(t, h, id, commit)
}
