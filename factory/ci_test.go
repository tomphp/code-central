package factory_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// pushRemoteURL is the Push remote URL the Factory last reported on the
// harness's Project.
func pushRemoteURL(t *testing.T, h *factorytest.Harness) string {
	t.Helper()
	p, err := h.Human().GetProject(context.Background(), h.Project)
	if err != nil {
		t.Fatal(err)
	}
	return p.CI.PushRemoteURL
}

func TestTheFactoryReportsThePushRemoteURLWhenItStarts(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	h.Git("remote", "set-url", "origin", "git@github.com:acme/web.git")

	h.Start()
	if got := pushRemoteURL(t, h); got != "git@github.com:acme/web.git" {
		t.Fatalf("push remote URL %q, want origin's", got)
	}
}

func TestThePushRemoteReportedIsTheOneTheLandingMethodNames(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	h.AddRemote("upstream")
	h.Git("remote", "set-url", "upstream", "https://github.com/acme/web.git")
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingMerge, Push: true, Remote: "upstream"})

	h.Start()
	if got := pushRemoteURL(t, h); got != "https://github.com/acme/web.git" {
		t.Fatalf("push remote URL %q, want upstream's", got)
	}
}

func TestAFactoryWithNoPushRemoteReportsNothingAndStillStarts(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Still claimed")
	h.Script(id, factorytest.Wait())

	h.Start()
	h.WaitForState(id, store.InProgress)
	if got := pushRemoteURL(t, h); got != "" {
		t.Fatalf("push remote URL %q, want none reported", got)
	}
}

// trackCI turns CI Tracking on for the harness's Project as the human.
func trackCI(t *testing.T, h *factorytest.Harness) {
	t.Helper()
	if _, err := h.Human().SetCITracking(context.Background(), h.Project, core.SetCITrackingInput{Tracking: true, Repository: "acme/web"}); err != nil {
		t.Fatal(err)
	}
}

func TestUnderMergeAndPushTheFactoryReportsThePushedBaseAsTheTicketsPushedCommit(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	mergeAndPush(h, "origin")
	trackCI(t, h)
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	tk := h.WaitForState(id, store.Done)
	r.WaitForOutput(id + ": CI tracks main at ")
	merge := h.Git("rev-parse", "main")
	if tk.PushedCommit == nil || tk.PushedCommit.Commit != merge {
		t.Fatalf("pushed commit %+v, want the pushed merge %s", tk.PushedCommit, merge)
	}
	if tk.CIResult != core.CIRunning {
		t.Fatalf("ci result %q, want running with no runs reported", tk.CIResult)
	}
}

func TestUnderBranchAndPushTheFactoryReportsThePushedBranchAsTheTicketsPushedCommit(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingBranch, Push: true, Remote: "origin"})
	trackCI(t, h)
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	r.WaitForOutput(id + ": CI tracks " + id + " at ")
	r.Poll()
	tk := h.Ticket(id)
	if tk.State != store.Accepted || tk.PushedCommit == nil || tk.PushedCommit.Commit != commit {
		t.Fatalf("%s with pushed commit %+v, want Accepted with the pushed branch's %s", tk.State, tk.PushedCommit, commit)
	}
	if n := strings.Count(r.Stdout(), id+": CI tracks"); n != 1 {
		t.Fatalf("reported %d times, want once however often it polls:\n%s", n, r.Stdout())
	}
}

func TestWithCITrackingOffThePushedCommitIsNotReported(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	mergeAndPush(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")

	h.Start()
	if tk := h.WaitForState(id, store.Done); tk.PushedCommit != nil {
		t.Fatalf("pushed commit %+v, want none with CI Tracking off", tk.PushedCommit)
	}
}

func TestABranchPushedBeforeCITrackingWasOnIsNotTracked(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingBranch, Push: true, Remote: "origin"})
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	r.WaitForOutput(id + ": pushed " + id + " to origin")
	trackCI(t, h)
	r.Poll()
	r.Poll()
	if tk := h.Ticket(id); tk.PushedCommit != nil {
		t.Fatalf("pushed commit %+v, want none for a push made before CI Tracking was on", tk.PushedCommit)
	}
}
