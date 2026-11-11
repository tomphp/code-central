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
	reportAttempt(t, h, id, 1, ticket, commit, status, conclusion)
}

// reportAttempt reports, as the CI Watcher, attempt of run id of ci.yml
// on commit tracked against ticket, with its status and conclusion.
func reportAttempt(t *testing.T, h *factorytest.Harness, id int64, attempt int, ticket, commit string, status store.CIRunStatus, conclusion store.CIConclusion) {
	t.Helper()
	started := h.Clock.Now()
	run := core.CIRunView{ID: id, Attempt: attempt, Workflow: "ci.yml", Name: "CI", Branch: "main", Commit: commit,
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

func TestUnderMergeAndPushWithCITrackingACancelledCIResultDoesNotLand(t *testing.T) {
	t.Parallel()
	h, _, r := gatedHarness(t)
	id, pushed := gated(t, h, r, "Add a feature", "feature.txt")

	reportRun(t, h, 1, id, pushed, store.CICompleted, store.CICancelled)
	stillAccepted(t, h, r, id, "with a cancelled CI result")
	r.WaitForOutput(id + ": CI cancelled at " + pushed[:12])
}

// Land anyway (code-central-261): the human accepts the failed CI
// result, and the Factory lands the Ticket at its pushed commit on its
// next poll.
func TestAfterLandAnywayAFailedTicketLandsOnTheNextPoll(t *testing.T) {
	t.Parallel()
	h, _, r := gatedHarness(t)
	id, pushed := gated(t, h, r, "Add a feature", "feature.txt")
	reportRun(t, h, 1, id, pushed, store.CICompleted, store.CIFailure)
	stillAccepted(t, h, r, id, "with a failed CI result")

	if _, err := h.Server.Core.LandAnyway(core.WithActor(context.Background(), core.Human), h.Project, core.TicketInput{Ticket: core.Ref(id)}); err != nil {
		t.Fatal(err)
	}
	r.Poll()
	tk := h.WaitForState(id, store.Done)
	if m := tk.Moves[len(tk.Moves)-1]; m.Commit != pushed {
		t.Errorf("landed with %q, want the pushed commit %s", m.Commit, pushed)
	}
	r.WaitForOutput(id + ": landing anyway at " + pushed[:12])
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

// Send back to fix (code-central-264): the human sends a CI failed Ticket
// back, the next Claim fixes it on the same branch in the same worktree,
// and its re-accept pushes a newer commit, whose CI result replaces the
// failed one.
func TestASentBackCIFailureIsFixedOnItsBranchAndItsNewPushedCommitDecides(t *testing.T) {
	t.Parallel()
	h, remote, r := gatedHarness(t)
	id, failed := sentBackCIFailure(t, h, r)
	r.Poll()
	h.WaitForState(id, store.InReview)
	if calls := h.Calls(); len(calls) != 2 || calls[1].Ticket != id || calls[1].Dir != h.Worktree(id) {
		t.Fatalf("calls %+v, want the second Claim run in %s", calls, h.Worktree(id))
	}
	if got := h.Git("log", "-1", "--format=%s", id); got != "Fix CI" {
		t.Fatalf("%s's tip is %q, want the fix on the same branch", id, got)
	}

	h.Accept(id)
	r.Poll()
	h.WaitFor(id+"'s newer pushed commit", func() bool {
		pc := h.Ticket(id).PushedCommit
		return pc != nil && pc.Commit != failed
	})
	tk := h.Ticket(id)
	fixed := tk.PushedCommit.Commit
	if tk.CIResult != core.CIRunning || remote.Git("rev-parse", "main") != fixed {
		t.Fatalf("ci result %q, remote main %s: want the fix pushed as %s and its CI running", tk.CIResult, remote.Git("rev-parse", "main"), fixed)
	}
	stillAccepted(t, h, r, id, "while its new pushed commit's CI runs")

	reportRun(t, h, 2, id, fixed, store.CICompleted, store.CISuccess)
	r.Poll()
	if tk := h.WaitForState(id, store.Done); tk.Moves[len(tk.Moves)-1].Commit != fixed {
		t.Errorf("landed with %q, want the new pushed commit %s", tk.Moves[len(tk.Moves)-1].Commit, fixed)
	}
}

// sentBackCIFailure has the Factory work a Ticket, which the human
// accepts and the Factory pushes; its CI fails, and the human sends it
// back with the next Claim scripted to fix it on its branch. It returns
// the Ticket, Ready, and its failed pushed commit.
func sentBackCIFailure(t *testing.T, h *factorytest.Harness, r *factorytest.Running) (id, failed string) {
	t.Helper()
	id = h.ReadyTicket("Add a feature")
	h.Script(id,
		factorytest.WriteFile("feature.txt", "feature\n"),
		factorytest.Git("add", "feature.txt"),
		factorytest.Git("commit", "-q", "-m", "Add a feature"),
		factorytest.CLI("finish", "{{ticket}}", "--claim", "{{claim}}", "--summary", "Added.", "--branch", "{{ticket}}"),
		factorytest.Result("success"))
	r.Poll()
	h.WaitForState(id, store.InReview)
	h.Accept(id)
	r.Poll()
	r.WaitForOutput(id + ": CI tracks main at ")
	failed = h.Ticket(id).PushedCommit.Commit
	reportRun(t, h, 1, id, failed, store.CICompleted, store.CIFailure)
	stillAccepted(t, h, r, id, "with a failed CI result")

	note := "CI failed at " + failed[:12] + ": https://github.com/acme/web/actions/runs/1, failed jobs: test."
	if _, err := h.Human().SendBack(context.Background(), h.Project, core.SendBackInput{Ticket: core.Ref(id), Note: note}); err != nil {
		t.Fatal(err)
	}
	h.Script(id,
		factorytest.NeedFile("feature.txt"),
		factorytest.WriteFile("fix.txt", "fix\n"),
		factorytest.Git("add", "fix.txt"),
		factorytest.Git("commit", "-q", "-m", "Fix CI"),
		factorytest.CLI("finish", "{{ticket}}", "--claim", "{{claim}}", "--summary", "Fixed.", "--branch", "{{ticket}}"),
		factorytest.Result("success"))
	return id, failed
}

// New fix Ticket (code-central-264): a Ready Ticket on the same Spec
// carries the failure, and the failed Ticket lands on the next poll.
func TestAfterNewFixTicketTheFailedTicketLandsOnTheNextPoll(t *testing.T) {
	t.Parallel()
	h, _, r := gatedHarness(t)
	id, pushed := gated(t, h, r, "Add a feature", "feature.txt")
	reportRun(t, h, 1, id, pushed, store.CICompleted, store.CIFailure)
	stillAccepted(t, h, r, id, "with a failed CI result")

	res, err := h.Server.Core.NewFixTicket(core.WithActor(context.Background(), core.Human), h.Project, core.NewFixTicketInput{Ticket: core.Ref(id), Note: "Fix test."})
	if err != nil {
		t.Fatal(err)
	}
	h.Script(res.ID, factorytest.Wait())
	r.Poll()
	if tk := h.WaitForState(id, store.Done); tk.Moves[len(tk.Moves)-1].Commit != pushed {
		t.Errorf("landed with %q, want the pushed commit %s", tk.Moves[len(tk.Moves)-1].Commit, pushed)
	}
	if fix := h.Ticket(res.ID); fix.Spec.ID != h.Ticket(id).Spec.ID {
		t.Errorf("fix Ticket on %s, want the failed Ticket's Spec", fix.Spec.ID)
	}
}
