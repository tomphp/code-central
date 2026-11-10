package factory_test

import (
	"context"
	"slices"
	"testing"
	"time"

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

// Each failure option resolves the hold its own way (code-central-300).

// failedBlocker is a gated Ticket whose CI failed, and a dependant that
// is Ready and blocked by it, with Unblock on push on. The Blocker shows
// in Needs Me as CI failed with all four failure options.
func failedBlocker(t *testing.T) (h *factorytest.Harness, r *factorytest.Running, id, pushed, dep string) {
	t.Helper()
	h, _, r = gatedHarness(t)
	unblockOnPush(t, h)
	id, pushed = gated(t, h, r, "Add a feature", "feature.txt")
	reportRun(t, h, 1, id, pushed, store.CICompleted, store.CIFailure)
	dep = dependant(t, h, "Build on the feature", id)
	stillBlocked(t, h, r, dep, id, "with its CI failed")
	if reason := h.Ticket(id).NeedsMeReason; reason != core.ReasonCIFailed {
		t.Fatalf("%s is in Needs Me as %q, want %q", id, reason, core.ReasonCIFailed)
	}
	want := []string{core.OptionRerunCI, core.OptionLandAnyway, core.OptionSendBack, core.OptionNewFixTicket}
	if got := h.Ticket(id).CIFailureOptions; !slices.Equal(got, want) {
		t.Fatalf("%s offers %v, want all four failure options %v", id, got, want)
	}
	return h, r, id, pushed, dep
}

// stillBlocked fails unless dep is Ready and blocked by blocker after a
// poll.
func stillBlocked(t *testing.T, h *factorytest.Harness, r *factorytest.Running, dep, blocker, why string) {
	t.Helper()
	r.Poll()
	if tk := h.Ticket(dep); !tk.Blocked || tk.Claimable || tk.State != store.Ready {
		t.Fatalf("%s is %s, blocked %v, claimable %v: want it Ready and blocked by %s %s", dep, tk.State, tk.Blocked, tk.Claimable, blocker, why)
	}
}

// claimable fails unless dep is claimable.
func claimable(t *testing.T, h *factorytest.Harness, dep, why string) {
	t.Helper()
	if tk := h.Ticket(dep); tk.Blocked || !tk.Claimable {
		t.Fatalf("%s is blocked %v, claimable %v: want it claimable %s", dep, tk.Blocked, tk.Claimable, why)
	}
}

func TestUnderUnblockOnPushRerunFailedJobsClearsTheBlockerAgain(t *testing.T) {
	t.Parallel()
	h, r, id, pushed, dep := failedBlocker(t)

	if _, err := h.Server.Core.RerunCI(core.WithActor(context.Background(), core.Human), h.Project, core.TicketInput{Ticket: core.Ref(id)}); err != nil {
		t.Fatal(err)
	}
	claimable(t, h, dep, "while the Re-run request is pending")
	reportAttempt(t, h, 1, 2, id, pushed, store.CIInProgress, "")
	claimable(t, h, dep, "while the re-run is running")
	r.Poll()
	h.WaitForState(dep, store.InProgress)
	stillAccepted(t, h, r, id, "while its re-run runs")

	reportAttempt(t, h, 1, 2, id, pushed, store.CICompleted, store.CISuccess)
	r.Poll()
	h.WaitForState(id, store.Done)
}

func TestUnderUnblockOnPushLandAnywayClearsTheBlockerOnceItLands(t *testing.T) {
	t.Parallel()
	h, r, id, _, dep := failedBlocker(t)

	if _, err := h.Server.Core.LandAnyway(core.WithActor(context.Background(), core.Human), h.Project, core.TicketInput{Ticket: core.Ref(id)}); err != nil {
		t.Fatal(err)
	}
	if tk := h.Ticket(dep); !tk.Blocked {
		t.Fatalf("%s is unblocked by Land anyway on its own, want it blocked until %s lands", dep, id)
	}
	r.Poll()
	h.WaitForState(id, store.Done)
	r.Poll()
	h.WaitForState(dep, store.InProgress)
}

func TestUnderUnblockOnPushNewFixTicketClearsTheBlockerOnceItLands(t *testing.T) {
	t.Parallel()
	h, r, id, _, dep := failedBlocker(t)

	res, err := h.Server.Core.NewFixTicket(core.WithActor(context.Background(), core.Human), h.Project, core.NewFixTicketInput{Ticket: core.Ref(id), Note: "Fix test."})
	if err != nil {
		t.Fatal(err)
	}
	h.Script(res.ID, factorytest.Wait())
	if tk := h.Ticket(dep); !tk.Blocked {
		t.Fatalf("%s is unblocked by New fix Ticket on its own, want it blocked until %s lands", dep, id)
	}
	r.Poll()
	h.WaitForState(id, store.Done)
	r.Poll()
	h.WaitForState(dep, store.InProgress)
}

func TestUnderUnblockOnPushASentBackBlockerBlocksUntilItsNewPushsCIRuns(t *testing.T) {
	t.Parallel()
	h, _, r := gatedHarness(t)
	unblockOnPush(t, h)
	id, failed := sentBackCIFailure(t, h, r)
	h.Server.Clock.Advance(time.Minute) // fixing it takes time
	dep := dependant(t, h, "Build on the feature", id)
	stillBlocked(t, h, r, dep, id, "while it is sent back")
	h.WaitForState(id, store.InReview)
	stillBlocked(t, h, r, dep, id, "while it is In Review again")

	// An older push going green, say a late re-run, doesn't clear it:
	// only its new push counts.
	reportAttempt(t, h, 1, 2, id, failed, store.CICompleted, store.CISuccess)
	h.Accept(id)
	if tk := h.Ticket(dep); !tk.Blocked {
		t.Fatalf("%s is unblocked by %s's older green push, want it blocked until its new push", dep, id)
	}

	r.Poll()
	h.WaitFor(id+"'s newer pushed commit", func() bool {
		pc := h.Ticket(id).PushedCommit
		return pc != nil && pc.Commit != failed
	})
	if tk := h.Ticket(id); tk.CIResult != core.CIRunning {
		t.Fatalf("%s's CI result is %q, want its new push's running", id, tk.CIResult)
	}
	r.Poll()
	h.WaitForState(dep, store.InProgress)
	stillAccepted(t, h, r, id, "while its new push's CI runs")
}

func TestUnderUnblockOnPushAGatedBlockerWithNoWordOfItsCIIsStillCIUnavailable(t *testing.T) {
	t.Parallel()
	h, _, r := gatedHarness(t)
	unblockOnPush(t, h)
	id, _ := gated(t, h, r, "Add a feature", "feature.txt")
	dep := dependant(t, h, "Build on the feature", id)
	r.Poll()
	h.WaitForState(dep, store.InProgress)

	h.Server.Clock.Advance(core.CIUnavailableAfter)
	if reason := h.Ticket(id).NeedsMeReason; reason != core.ReasonCIUnavailable {
		t.Fatalf("%s is in Needs Me as %q, want %q though %s has moved on", id, reason, core.ReasonCIUnavailable, dep)
	}
}
