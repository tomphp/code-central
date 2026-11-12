package factory_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// holdTicketsStage adds a Draft tickets Stage Ticket, which keeps the Spec
// from being ready to land until it is cancelled; release cancels it.
func holdTicketsStage(t *testing.T, h *factorytest.Harness) (release func()) {
	t.Helper()
	ctx := context.Background()
	made, err := h.Server.Client.CreateTicket(ctx, h.Project, core.CreateTicketInput{Spec: core.Ref(h.SpecID()), Title: "Tickets", State: store.Draft, Labels: []string{core.StageLabel("tickets")}})
	if err != nil {
		t.Fatal(err)
	}
	return func() {
		if _, err := h.Human().Cancel(ctx, h.Project, core.CancelInput{Ticket: core.Ref(made.ID)}); err != nil {
			t.Fatal(err)
		}
	}
}

// twoAccepted is a Requirement Spec with two Accepted Tickets and its
// tickets Stage Done.
func twoAccepted(t *testing.T, h *factorytest.Harness) (a, b string) {
	t.Helper()
	h.MakeRequirement()
	a, _ = accepted(h, "First", "one.txt", "1\n")
	b, _ = accepted(h, "Second", "two.txt", "2\n")
	return a, b
}

func TestAReadySpecIsMergedIntoTheBaseAndItsTicketsLandAtTheMergeCommit(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	a, b := twoAccepted(t, h)

	r := h.Start()
	h.WaitForState(a, store.Done)
	tkB := h.WaitForState(b, store.Done)

	merge := h.Git("rev-parse", "main")
	if got := h.Git("log", "-1", "--format=%s", merge); got != "Merge spec "+h.SpecID()+": Work" {
		t.Errorf("merge subject %q", got)
	}
	if m := tkB.Moves[len(tkB.Moves)-1]; m.Commit != merge {
		t.Errorf("landed at %q, want the merge %s", m.Commit, merge)
	}
	if m := h.Ticket(a).Moves; m[len(m)-1].Commit != merge {
		t.Errorf("first landed at %q, want the merge %s", m[len(m)-1].Commit, merge)
	}
	r.WaitForOutput(h.SpecID() + ": merged spec/" + h.SpecID() + " into main at ")
	r.WaitForOutput(a + ": landed on main at ")
}

func TestNothingLandsWhileTheSpecIsNotReady(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	a, _ := twoAccepted(t, h)
	release := holdTicketsStage(t, h)
	r := h.Start()
	r.WaitForOutput(a + ": merged into spec/")
	r.Poll()
	if h.Ticket(a).State != store.Accepted {
		t.Error("landed before the tickets Stage was finished")
	}
	release()

	later := h.ReadyTicket("Added later")
	h.Script(later, factorytest.Wait())
	r.Poll()
	h.WaitForState(later, store.InProgress)
	r.Poll()
	r.Poll()
	if h.Ticket(a).State != store.Accepted {
		t.Error("landed with a Ticket In Progress")
	}
}

func TestUnderPushASpecIsPushedOnceAndItsTicketsLand(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	a, b := twoAccepted(t, h)

	r := h.Start()
	h.WaitForState(a, store.Done)
	h.WaitForState(b, store.Done)
	if got, want := remote.Git("rev-parse", "main"), h.Git("rev-parse", "main"); got != want {
		t.Errorf("the remote's main is %s, want %s", got, want)
	}
	if got := remote.Git("branch", "--list"); got != "main" && got != "* main" {
		t.Errorf("the remote has branches %q, want only main", got)
	}
	_ = r
}

func TestAFailedPushLeavesTheSpecsTicketsAcceptedAndRetries(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")
	remote.RefusePushes()
	mergeAndPush(h, "origin")
	a, b := twoAccepted(t, h)

	r := h.Start()
	r.Poll()
	for _, id := range []string{a, b} {
		tk := h.Ticket(id)
		if tk.State != store.Accepted || !commented(tk, "couldn't push main to origin") {
			t.Errorf("%s is %s with comments %+v, want Accepted and commented", id, tk.State, tk.Comments)
		}
	}
	if got := remote.Git("rev-parse", "main"); got != theirs {
		t.Errorf("the remote moved to %s", got)
	}

	remote.AcceptPushes()
	r.Poll()
	h.WaitForState(a, store.Done)
	h.WaitForState(b, store.Done)
}

func TestWithCITrackingASpecsTicketsShareOnePushedCommitAndLandOncePassed(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	mergeAndPush(h, "origin")
	trackCI(t, h)
	a, b := twoAccepted(t, h)

	r := h.Start()
	r.WaitForOutput(b + ": CI tracks main at ")
	r.WaitForOutput(a + ": CI tracks main at ")
	pa, pb := h.Ticket(a).PushedCommit, h.Ticket(b).PushedCommit
	if pa == nil || pb == nil || pa.Commit != pb.Commit {
		t.Fatalf("pushed commits %+v and %+v, want the same one", pa, pb)
	}
	stillAccepted(t, h, r, a, "while CI hasn't passed")

	// One shared run per commit, as the real CI Watcher reports, tracked
	// against one of the Tickets; both read it.
	reportRun(t, h, 1, a, pa.Commit, store.CICompleted, store.CIFailure)
	stillAccepted(t, h, r, a, "with a failed CI result")
	stillAccepted(t, h, r, b, "with a failed CI result")

	// A re-run passes.
	reportAttempt(t, h, 1, 2, a, pa.Commit, store.CICompleted, store.CISuccess)
	r.Poll()
	h.WaitForState(a, store.Done)
	h.WaitForState(b, store.Done)
}

func TestASpecBranchThatConflictsWithTheBaseOpensOneConflictTicket(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	a, _ := accepted(h, "First", "README", "spec side\n")
	release := holdTicketsStage(t, h)
	r := h.Start()
	r.WaitForOutput(a + ": merged into spec/" + h.SpecID())

	if err := os.WriteFile(filepath.Join(h.Repo, "README"), []byte("base side\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.Git("add", "README")
	h.Git("commit", "-q", "-m", "base changes README")
	release()
	r.Poll()
	r.WaitForOutput(h.SpecID() + ": spec/" + h.SpecID() + " conflicts with main in README: opened ")
	r.Poll()

	rows, err := h.Server.Client.FindTickets(context.Background(), h.Project, core.FindTicketsInput{Spec: core.Ref(h.SpecID()), Title: "Merge the base into the Spec Branch of " + h.SpecID()})
	if err != nil || len(rows) != 1 {
		t.Errorf("conflict Tickets %+v (%v), want exactly one", rows, err)
	}
	if h.Ticket(a).State != store.Accepted {
		t.Errorf("%s landed despite the conflict", a)
	}
}

func TestOnceTheConflictTicketIsAcceptedTheSpecLandsAgain(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	a, _ := accepted(h, "First", "README", "spec side\n")
	release := holdTicketsStage(t, h)
	// The conflict Ticket is the next one made: its session merges the
	// base into the Spec Branch and finishes.
	fix := "acme-5"
	h.Script(fix, factorytest.Git("merge", "-q", "-X", "theirs", "main"), factorytest.CLI("finish", "{{ticket}}", "--claim", "{{claim}}", "--summary", "Merged the base.", "--branch", fix))
	r := h.Start()
	r.WaitForOutput(a + ": merged into spec/" + h.SpecID())

	if err := os.WriteFile(filepath.Join(h.Repo, "README"), []byte("base side\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.Git("add", "README")
	h.Git("commit", "-q", "-m", "base changes README")
	release()
	r.Poll()
	r.WaitForOutput(h.SpecID() + ": spec/" + h.SpecID() + " conflicts with main in README: opened " + fix)
	h.WaitForState(fix, store.InReview)

	h.Accept(fix)
	r.Poll()
	h.WaitForState(a, store.Done)
	h.WaitForState(fix, store.Done)
}
