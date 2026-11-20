package factory_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

// holdTicketsStage adds a Draft tickets Stage Ticket, which keeps the Spec
// from being ready to land until it is cancelled; release cancels it.
func holdTicketsStage(t *testing.T, h *factorytest.Harness) (release func()) {
	t.Helper()
	ctx := context.Background()
	made, err := h.Server.Client.CreateTicket(ctx, h.Project, api.CreateTicketInput{Spec: api.Ref(h.SpecID()), Title: "Tickets", State: domain.Draft, Labels: []string{domain.StageLabel("tickets")}})
	if err != nil {
		t.Fatal(err)
	}
	return func() {
		if _, err := h.Human().Cancel(ctx, h.Project, api.CancelInput{Ticket: api.Ref(made.ID)}); err != nil {
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
	h.WantState(a, domain.Done)
	tkB := h.WantState(b, domain.Done)

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
	r.WantOutput(h.SpecID() + ": merged spec/" + h.SpecID() + " into main at ")
	r.WantOutput(a + ": landed on main at ")
}

func TestNothingLandsWhileTheSpecIsNotReady(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	a, _ := twoAccepted(t, h)
	release := holdTicketsStage(t, h)
	r := h.Start()
	r.WantOutput(a + ": merged into spec/")
	r.Poll()
	if h.Ticket(a).State != domain.Accepted {
		t.Error("landed before the tickets Stage was finished")
	}
	release()

	later := h.ReadyTicket("Added later")
	h.Script(later, factorytest.Wait())
	r.Poll()
	h.WantState(later, domain.InProgress)
	r.Poll()
	r.Poll()
	if h.Ticket(a).State != domain.Accepted {
		t.Error("landed with a Ticket In Progress")
	}
}

func TestUnderPushASpecIsPushedOnceAndItsTicketsLand(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	a, b := twoAccepted(t, h)

	h.Start()
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
	if got, want := remote.Git("rev-parse", "main"), h.Git("rev-parse", "main"); got != want {
		t.Errorf("the remote's main is %s, want %s", got, want)
	}
	if got := remote.Git("branch", "--list"); got != "main" && got != "* main" {
		t.Errorf("the remote has branches %q, want only main", got)
	}
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
		if tk.State != domain.Accepted || !commented(tk, "couldn't push main to origin") {
			t.Errorf("%s is %s with comments %+v, want Accepted and commented", id, tk.State, tk.Comments)
		}
	}
	if got := remote.Git("rev-parse", "main"); got != theirs {
		t.Errorf("the remote moved to %s", got)
	}

	remote.AcceptPushes()
	r.Poll()
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
}

func TestWithCITrackingASpecsTicketsShareOnePushedCommitAndLandOncePassed(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	mergeAndPush(h, "origin")
	trackCI(t, h)
	a, b := twoAccepted(t, h)

	r := h.Start()
	r.WantOutput(b + ": CI tracks main at ")
	r.WantOutput(a + ": CI tracks main at ")
	pa, pb := h.Ticket(a).PushedCommit, h.Ticket(b).PushedCommit
	if pa == nil || pb == nil || pa.Commit != pb.Commit {
		t.Fatalf("pushed commits %+v and %+v, want the same one", pa, pb)
	}
	stillAccepted(t, h, r, a, "while CI hasn't passed")

	// One shared run per commit, as the real CI Watcher reports, tracked
	// against one of the Tickets; both read it.
	reportRun(t, h, 1, a, pa.Commit, domain.CICompleted, domain.CIFailure)
	stillAccepted(t, h, r, a, "with a failed CI result")
	stillAccepted(t, h, r, b, "with a failed CI result")

	// A re-run passes.
	reportAttempt(t, h, 1, 2, a, pa.Commit, domain.CICompleted, domain.CISuccess)
	r.Poll()
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
}

func TestASpecBranchThatConflictsWithTheBaseOpensOneConflictTicket(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	a, _ := accepted(h, "First", "README", "spec side\n")
	release := holdTicketsStage(t, h)
	r := h.Start()
	r.WantOutput(a + ": merged into spec/" + h.SpecID())

	if err := os.WriteFile(filepath.Join(h.Repo, "README"), []byte("base side\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.Git("add", "README")
	h.Git("commit", "-q", "-m", "base changes README")
	release()
	r.Poll()
	r.WantOutput(h.SpecID() + ": spec/" + h.SpecID() + " conflicts with main in README: opened ")
	r.Poll()

	rows, err := h.Server.Client.FindTickets(context.Background(), h.Project, api.FindTicketsInput{Spec: api.Ref(h.SpecID()), Title: "Merge the base into the Spec Branch of " + h.SpecID()})
	if err != nil || len(rows) != 1 {
		t.Fatalf("conflict Tickets %+v (%v), want exactly one", rows, err)
	}
	// Code Central opened it, marked, from the files and both tips.
	fix := h.Ticket(rows[0].ID)
	baseTip, specTip := h.Git("rev-parse", "main"), h.Git("rev-parse", "spec/"+h.SpecID())
	if !fix.SpecMergeTicket || !strings.Contains(fix.Description, "- README\n") ||
		!strings.Contains(fix.Description, "base's tip is "+baseTip) || !strings.Contains(fix.Description, "Spec Branch's tip is "+specTip) {
		t.Errorf("conflict Ticket %+v, want it marked, naming README, %s and %s", fix, baseTip, specTip)
	}
	if h.Ticket(a).State != domain.Accepted {
		t.Errorf("%s landed despite the conflict", a)
	}
}

func TestASpecBranchConflictingWithAMovedBaseCommentsOnItsOpenSpecMergeTicket(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	a, _ := accepted(h, "First", "README", "spec side\n")
	release := holdTicketsStage(t, h)
	fix := "acme-5"
	h.Script(fix, factorytest.Wait())
	r := h.Start()
	r.WantOutput(a + ": merged into spec/" + h.SpecID())

	h.CommitOn("main", "README", "base side\n")
	release()
	r.Poll()
	r.WantOutput(h.SpecID() + ": spec/" + h.SpecID() + " conflicts with main in README: opened " + fix)

	// Land per Spec off merges the Spec Branch on every poll, ready or
	// not, so the moved base meets the same conflict.
	h.CommitOn("main", "README", "base side again\n")
	baseTip := h.Git("rev-parse", "main")
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingMerge, LandPerSpec: off()})
	r.Poll()
	r.WantOutput(h.SpecID() + ": spec/" + h.SpecID() + " conflicts with main again: commented on " + fix)

	comments := h.Ticket(fix).Comments
	if len(comments) != 1 || !strings.Contains(comments[0].Body, "base's tip is "+baseTip) {
		t.Errorf("comments %+v, want one naming the base's tip %s", comments, baseTip)
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
	r.WantOutput(a + ": merged into spec/" + h.SpecID())

	if err := os.WriteFile(filepath.Join(h.Repo, "README"), []byte("base side\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.Git("add", "README")
	h.Git("commit", "-q", "-m", "base changes README")
	release()
	r.Poll()
	r.WantOutput(h.SpecID() + ": spec/" + h.SpecID() + " conflicts with main in README: opened " + fix)
	h.WantState(fix, domain.InReview)

	h.Accept(fix)
	r.Poll()
	h.WantState(a, domain.Done)
	h.WantState(fix, domain.Done)
}
