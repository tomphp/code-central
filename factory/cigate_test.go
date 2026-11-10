package factory_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// The gate (code-central-257): under Merge + Push with CI Tracking on, a
// Ticket lands only once its pushed commit's CI result is passed.

// gated pushes an Accepted Ticket's merge under Merge + Push with CI
// Tracking on, and waits until it is reported as the Ticket's pushed
// commit. It returns the Ticket and the pushed commit.
func gated(t *testing.T, h *factorytest.Harness, r *factorytest.Running, title, file string) (id, pushed string) {
	t.Helper()
	id, _ = accepted(h, title, file, title+"\n")
	r.Poll()
	r.WaitForOutput(id + ": CI tracks main at ")
	tk := h.Ticket(id)
	if tk.State != store.Accepted || tk.PushedCommit == nil {
		t.Fatalf("%s with pushed commit %+v, want it pushed and left Accepted", tk.State, tk.PushedCommit)
	}
	return id, tk.PushedCommit.Commit
}

// gatedHarness is a harness under Merge + Push to origin with CI Tracking
// on, its Factory started.
func gatedHarness(t *testing.T) (*factorytest.Harness, *factorytest.Remote, *factorytest.Running) {
	t.Helper()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	trackCI(t, h)
	return h, remote, h.Start()
}

// reportRun reports, as the CI Watcher, one run of ci.yml on commit
// tracked against ticket, with its status and conclusion.
func reportRun(t *testing.T, h *factorytest.Harness, id int64, ticket, commit string, status store.CIRunStatus, conclusion store.CIConclusion) {
	t.Helper()
	started := h.Clock.Now()
	run := core.CIRunView{ID: id, Attempt: 1, Workflow: "ci.yml", Name: "CI", Branch: "main", Commit: commit,
		Status: status, Conclusion: conclusion, Started: &started, URL: "https://github.com/acme/web/actions/runs/1", Jobs: []core.CIJobView{}, Ticket: ticket}
	if _, err := h.Server.Client.ReportCIRuns(context.Background(), h.Project, core.ReportCIRunsInput{Runs: []core.CIRunView{run}}); err != nil {
		t.Fatal(err)
	}
}

// stillAccepted fails unless ticket is Accepted after a poll.
func stillAccepted(t *testing.T, h *factorytest.Harness, r *factorytest.Running, ticket, why string) {
	t.Helper()
	r.Poll()
	if s := h.Ticket(ticket).State; s != store.Accepted {
		t.Fatalf("%s is %s, want it left Accepted %s", ticket, s, why)
	}
}

func TestUnderMergeAndPushWithCITrackingATicketLandsAtItsPushedCommitOncePassed(t *testing.T) {
	t.Parallel()
	h, remote, r := gatedHarness(t)
	id, pushed := gated(t, h, r, "Add a feature", "feature.txt")
	if got := remote.Git("rev-parse", "main"); got != pushed {
		t.Fatalf("the remote's main is at %q, want the pushed commit %s", got, pushed)
	}
	stillAccepted(t, h, r, id, "with no CI Runs reported")

	reportRun(t, h, 1, id, pushed, store.CIInProgress, "")
	stillAccepted(t, h, r, id, "while its run is running")

	reportRun(t, h, 1, id, pushed, store.CICompleted, store.CISuccess)
	r.Poll()
	tk := h.WaitForState(id, store.Done)
	if m := tk.Moves[len(tk.Moves)-1]; m.Commit != pushed {
		t.Errorf("landed with %q, want the pushed commit %s", m.Commit, pushed)
	}
	if n := strings.Count(r.Stdout(), id+": CI tracks"); n != 1 {
		t.Errorf("reported %d times, want once however often it polls:\n%s", n, r.Stdout())
	}
}

func TestUnderMergeAndPushWithCITrackingAFailedCIResultDoesNotLand(t *testing.T) {
	t.Parallel()
	h, _, r := gatedHarness(t)
	id, pushed := gated(t, h, r, "Add a feature", "feature.txt")

	reportRun(t, h, 1, id, pushed, store.CICompleted, store.CIFailure)
	stillAccepted(t, h, r, id, "with a failed CI result")
	r.WaitForOutput(id + ": CI failed at " + pushed[:12])
}

func TestATicketBlockedByAGatedTicketStaysBlockedUntilItLands(t *testing.T) {
	t.Parallel()
	h, _, r := gatedHarness(t)
	id, pushed := gated(t, h, r, "Add a feature", "feature.txt")
	blocked := h.ReadyTicket("Build on the feature")
	if _, err := h.Server.Client.AddBlocker(context.Background(), h.Project, core.BlockerInput{Ticket: core.Ref(blocked), Blocker: core.Ref(id)}); err != nil {
		t.Fatal(err)
	}
	h.Script(blocked, factorytest.Wait())

	stillAccepted(t, h, r, id, "with no CI Runs reported")
	if tk := h.Ticket(blocked); !tk.Blocked || tk.State != store.Ready {
		t.Fatalf("%s is %s, blocked %v: want it Ready and blocked while %s waits on CI", blocked, tk.State, tk.Blocked, id)
	}

	reportRun(t, h, 1, id, pushed, store.CICompleted, store.CISuccess)
	r.Poll()
	h.WaitForState(id, store.Done)
	r.Poll()
	h.WaitForState(blocked, store.InProgress)
}

func TestTheGateLiftingLandsAWaitingTicketOnTheNextPoll(t *testing.T) {
	t.Parallel()
	for name, lift := range map[string]func(*testing.T, *factorytest.Harness){
		"ci tracking off": func(t *testing.T, h *factorytest.Harness) {
			if _, err := h.Human().SetCITracking(context.Background(), h.Project, core.SetCITrackingInput{Tracking: false, Repository: "acme/web"}); err != nil {
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
			id, _ := gated(t, h, r, "Add a feature", "feature.txt")
			stillAccepted(t, h, r, id, "while the gate holds")

			lift(t, h)
			r.Poll()
			tk := h.Ticket(id)
			if tk.State != store.Done {
				t.Fatalf("%s is %s, want it landed on the poll after the gate lifts", id, tk.State)
			}
			if m := tk.Moves[len(tk.Moves)-1]; m.Commit == "" {
				t.Errorf("landed with no commit: %+v", m)
			}
		})
	}
}

func TestATicketWaitingOnCIDoesNotStopAnotherFromMergingAndPushing(t *testing.T) {
	t.Parallel()
	h, remote, r := gatedHarness(t)
	first, firstPushed := gated(t, h, r, "Add a feature", "feature.txt")
	second, secondPushed := gated(t, h, r, "Add another", "another.txt")
	if got := remote.Git("rev-parse", "main"); got != secondPushed {
		t.Fatalf("the remote's main is at %q, want the second Ticket's merge %s pushed", got, secondPushed)
	}
	stillAccepted(t, h, r, first, "while its own CI Run is running")
	if tk := h.Ticket(first); tk.PushedCommit == nil || tk.PushedCommit.Commit != firstPushed {
		t.Fatalf("%s's pushed commit is %+v, want its own %s, not the later push's", first, tk.PushedCommit, firstPushed)
	}

	reportRun(t, h, 2, second, secondPushed, store.CICompleted, store.CISuccess)
	r.Poll()
	h.WaitForState(second, store.Done)
	stillAccepted(t, h, r, first, "while its own CI Run is running")
}

func TestUnderBranchAndPushTheHumansMergeLandsWhateverTheCIResult(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingBranch, Push: true, Remote: "origin"})
	trackCI(t, h)
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	r.WaitForOutput(id + ": CI tracks " + id + " at ")
	reportRun(t, h, 1, id, commit, store.CICompleted, store.CIFailure)
	h.Git("merge", "-q", "--no-ff", "-m", "Merge "+id, id)
	r.Poll()
	if tk := h.WaitForState(id, store.Done); tk.CIResult != core.CIFailed {
		t.Fatalf("ci result %q, want it landed with its failed CI result", tk.CIResult)
	}
}

func TestAnInReviewTicketMergedByHandWaitsOnCIInReview(t *testing.T) {
	t.Parallel()
	h, remote, r := gatedHarness(t)
	id := h.ReadyTicket("Add a feature")
	commit := h.CommitOn(id, "feature.txt", "feature\n")
	h.Finished(id, id, commit)
	h.Git("merge", "-q", "--no-ff", "-m", "My own merge", id)

	r.Poll()
	r.WaitForOutput(id + ": CI tracks main at ")
	tk := h.Ticket(id)
	merge := h.Git("rev-parse", "main")
	if tk.State != store.InReview || tk.PushedCommit == nil || tk.PushedCommit.Commit != merge {
		t.Fatalf("%s with pushed commit %+v, want it In Review with the pushed merge %s", tk.State, tk.PushedCommit, merge)
	}
	if got := remote.Git("rev-parse", "main"); got != merge {
		t.Fatalf("the remote's main is at %q, want the merge %s", got, merge)
	}
	r.Poll()
	if s := h.Ticket(id).State; s != store.InReview {
		t.Fatalf("%s is %s, want it left In Review while CI runs", id, s)
	}

	reportRun(t, h, 1, id, merge, store.CICompleted, store.CISuccess)
	r.Poll()
	if tk := h.WaitForState(id, store.Done); tk.Moves[len(tk.Moves)-1].Commit != merge {
		t.Errorf("landed with %q, want the pushed merge %s", tk.Moves[len(tk.Moves)-1].Commit, merge)
	}
}
