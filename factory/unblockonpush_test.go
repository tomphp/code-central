package factory_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// Unblock on push (code-central-298): while the CI gate holds a pushed
// Ticket, it stops blocking once its CI result is running or passed, and
// blocks again if that result turns failed. It still lands only on green.

// unblockOnPush turns Unblock on push on, with CI Tracking on.
func unblockOnPush(t *testing.T, h *factorytest.Harness) {
	t.Helper()
	if _, err := h.Human().SetCITracking(context.Background(), h.Project, core.SetCITrackingInput{Tracking: true, Repository: "acme/web", UnblockOnPush: true}); err != nil {
		t.Fatal(err)
	}
}

// dependant creates a Ready Ticket blocked by blocker, whose run waits.
func dependant(t *testing.T, h *factorytest.Harness, title, blocker string) string {
	t.Helper()
	id := h.ReadyTicket(title)
	if _, err := h.Server.Client.AddBlocker(context.Background(), h.Project, core.BlockerInput{Ticket: core.Ref(id), Blocker: core.Ref(blocker)}); err != nil {
		t.Fatal(err)
	}
	h.Script(id, factorytest.Wait())
	return id
}

func TestUnderUnblockOnPushADependantIsClaimedWhileItsBlockersCIRuns(t *testing.T) {
	t.Parallel()
	for name, report := range map[string]func(t *testing.T, h *factorytest.Harness, id, pushed string){
		"no runs reported": func(*testing.T, *factorytest.Harness, string, string) {},
		"a run in progress": func(t *testing.T, h *factorytest.Harness, id, pushed string) {
			reportRun(t, h, 1, id, pushed, store.CIInProgress, "")
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, _, r := gatedHarness(t)
			unblockOnPush(t, h)
			id, pushed := gated(t, h, r, "Add a feature", "feature.txt")
			report(t, h, id, pushed)
			dep := dependant(t, h, "Build on the feature", id)

			r.Poll()
			h.WaitForState(dep, store.InProgress)
			stillAccepted(t, h, r, id, "while its CI runs")

			reportRun(t, h, 1, id, pushed, store.CICompleted, store.CISuccess)
			r.Poll()
			h.WaitForState(id, store.Done)
		})
	}
}

func TestUnderUnblockOnPushAFailedRunBlocksAnUnclaimedDependantAgainWhileAClaimedOneCarriesOn(t *testing.T) {
	t.Parallel()
	h, _, r := gatedHarness(t)
	unblockOnPush(t, h)
	id, pushed := gated(t, h, r, "Add a feature", "feature.txt")
	reportRun(t, h, 1, id, pushed, store.CIInProgress, "")
	claimed := dependant(t, h, "Build on the feature", id)
	r.Poll()
	h.WaitForState(claimed, store.InProgress)

	unclaimed := dependant(t, h, "Build more on the feature", id)
	if tk := h.Ticket(unclaimed); tk.Blocked || !tk.Claimable {
		t.Fatalf("%s blocked %v claimable %v, want it claimable while %s's CI runs", unclaimed, tk.Blocked, tk.Claimable, id)
	}

	reportRun(t, h, 1, id, pushed, store.CICompleted, store.CIFailure)
	if tk := h.Ticket(unclaimed); !tk.Blocked || tk.Claimable {
		t.Fatalf("%s blocked %v claimable %v, want it blocked again once %s's CI failed", unclaimed, tk.Blocked, tk.Claimable, id)
	}
	stillAccepted(t, h, r, id, "with a failed CI result")
	if s := h.Ticket(unclaimed).State; s != store.Ready {
		t.Fatalf("%s is %s, want it left Ready while %s's CI is failed", unclaimed, s, id)
	}
	if s := h.Ticket(claimed).State; s != store.InProgress {
		t.Fatalf("%s is %s, want the claimed dependant to carry on", claimed, s)
	}
}

func TestUnderUnblockOnPushADependantOfAPassedTicketIsClaimedBeforeItLands(t *testing.T) {
	t.Parallel()
	h, _, r := gatedHarness(t)
	unblockOnPush(t, h)
	id, pushed := gated(t, h, r, "Add a feature", "feature.txt")
	reportRun(t, h, 1, id, pushed, store.CICompleted, store.CISuccess)
	dep := dependant(t, h, "Build on the feature", id)
	if tk := h.Ticket(dep); tk.Blocked {
		t.Fatalf("%s is blocked by %s, Accepted with its CI passed, before the Factory lands it", dep, id)
	}
	r.Poll()
	h.WaitForState(id, store.Done)
	h.WaitForState(dep, store.InProgress)
}

func TestUnblockOnPushChangesNothingWithoutTheCIGate(t *testing.T) {
	t.Parallel()
	t.Run("branch and push", func(t *testing.T) {
		t.Parallel()
		h := factorytest.New(t)
		h.AddRemote("origin")
		h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingBranch, Push: true, Remote: "origin"})
		unblockOnPush(t, h)
		id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")
		r := h.Start()
		r.WaitForOutput(id + ": CI tracks " + id + " at ")
		reportRun(t, h, 1, id, commit, store.CIInProgress, "")
		dep := dependant(t, h, "Build on the feature", id)

		r.Poll()
		if tk := h.Ticket(dep); !tk.Blocked || tk.State != store.Ready {
			t.Fatalf("%s is %s, blocked %v: want it Ready and blocked until %s is merged by hand", dep, tk.State, tk.Blocked, id)
		}
	})
	for name, lift := range map[string]func(*testing.T, *factorytest.Harness){
		"ci tracking off": func(t *testing.T, h *factorytest.Harness) {
			if _, err := h.Human().SetCITracking(context.Background(), h.Project, core.SetCITrackingInput{Tracking: false, Repository: "acme/web", UnblockOnPush: true}); err != nil {
				t.Fatal(err)
			}
		},
		"push off": func(t *testing.T, h *factorytest.Harness) {
			h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingMerge})
		},
		"branch": func(t *testing.T, h *factorytest.Harness) {
			h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingBranch, Push: true, Remote: "origin"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, _, r := gatedHarness(t)
			unblockOnPush(t, h)
			id, pushed := gated(t, h, r, "Add a feature", "feature.txt")
			reportRun(t, h, 1, id, pushed, store.CIInProgress, "")
			dep := dependant(t, h, "Build on the feature", id)

			lift(t, h)
			if tk := h.Ticket(dep); !tk.Blocked {
				t.Fatalf("%s is unblocked by %s once the gate lifts, want it blocked until %s lands", dep, id, id)
			}
			r.Poll()
			h.WaitForState(id, store.Done)
			r.Poll()
			h.WaitForState(dep, store.InProgress)
		})
	}
}
