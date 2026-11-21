package factory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

// Under Merge + Push the Factory fetches the Push remote and brings the
// local base level with it before it lands or cuts a branch
// (code-central-428): behind, it fast-forwards; level or ahead, it is
// current; both moved, it moves the local base onto the remote's
// (movebase_test.go), and reports Base diverged when it can't.

func TestUnderMergeAndPushARemoteThatIsAheadIsFastForwardedBeforeTheTicketLands(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	// Pushed by path, so the repo's origin/main doesn't know of it.
	h.PushByHand(remote, "theirs", "main")
	mergeAndPush(h, "origin")
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	r.WantOutput("main: fast-forwarded to origin/main")
	tk := h.WantState(id, domain.Done)
	merge := h.Tip("main")
	if got := strings.Join(h.Parents(merge), " "); got != theirs+" "+commit {
		t.Errorf("main's tip has parents %q, want the remote's %s then %s", got, theirs, commit)
	}
	if got := remote.Tip("main"); got != merge {
		t.Errorf("the remote's main is at %q, want the merge %s on top of theirs", got, merge)
	}
	if m := tk.Moves[len(tk.Moves)-1]; m.Commit != merge {
		t.Errorf("landed with %q, want the merge %s", m.Commit, merge)
	}
}

func TestUnderMergeAndPushWithCITrackingARemoteThatIsAheadIsFastForwarded(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.PushByHand(remote, "theirs", "main")
	mergeAndPush(h, "origin")
	trackCI(t, h)
	ready := h.ReadyTicket("Starts from the remote's base")

	h.Start()
	h.WantCalls(1)
	if got := h.Tip("main"); got != theirs {
		t.Errorf("the local base is at %s, want it fast-forwarded to %s", got, theirs)
	}
	if !h.IsAncestor(theirs, ready) {
		t.Errorf("%s is not in %s", theirs, ready)
	}
}

func TestUnderMergeAndPushANewBranchIsCutFromTheFastForwardedBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	h.PushByHand(remote, "main", "main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.PushByHand(remote, "theirs", "main")
	ready := h.ReadyTicket("Starts from the remote's base")

	h.Start()
	calls := h.WantCalls(1)
	if got := h.Tip("main"); got != theirs {
		t.Errorf("the local base is at %s, want it fast-forwarded to %s", got, theirs)
	}
	if calls[0].Ticket != ready {
		t.Fatalf("calls %+v, want %s claimed", calls, ready)
	}
	if !h.IsAncestor(theirs, ready) {
		t.Errorf("%s is not in %s", theirs, ready)
	}
}

func TestUnderMergeAndPushALocalBaseAheadOfTheRemoteIsCurrent(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	h.PushByHand(remote, "main", "main")
	mine := h.CommitOn("main", "mine.txt", "mine\n")
	mergeAndPush(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	h.WantState(id, domain.Done)
	if _, ok := baseDivergedItem(t, h); ok {
		t.Error("an unpushed local commit showed Base diverged")
	}
	if !remote.IsAncestor(mine, "main") {
		t.Errorf("%s is not in the remote's main", mine)
	}
	if out := r.Stdout() + r.Stderr(); strings.Contains(out, "diverged") {
		t.Errorf("the Factory reported a divergence:\n%s", out)
	}
}

func TestUnderMergeAndPushAConflictingMoveLeavesTheBaseAsItWasAndStopsLandingAndClaiming(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	h.PushByHand(remote, "main", "main")
	theirs := h.CommitOn("theirs", "shared.txt", "theirs\n")
	h.PushByHand(remote, "theirs", "main")
	mine := h.CommitOn("main", "shared.txt", "mine\n")
	ready := h.ReadyTicket("Waits for the base")

	r := h.Start()
	r.WantOutput("warning: can't move main onto origin/main: " + mine[:12] + " conflicts in shared.txt")
	r.WantOutput("has diverged from the remote's")
	if got := h.Tip("main"); got != mine {
		t.Errorf("main moved to %s, want it left at %s", got, mine)
	}
	if tk := h.Ticket(id); tk.State != domain.Accepted {
		t.Errorf("%s is %s, want it held Accepted", id, tk.State)
	}
	if tk := h.Ticket(ready); tk.State != domain.Ready {
		t.Errorf("%s is %s, want it not claimed", ready, tk.State)
	}
	if got := remote.Tip("main"); got != theirs {
		t.Errorf("the remote's main is at %q, want it left at %s", got, theirs)
	}
	local := h.Tip("main")
	r.Poll()
	row, ok := baseDivergedItem(t, h)
	if !ok {
		t.Fatal("Base diverged doesn't show in Needs Me")
	}
	if got := row.BaseDiverged; got.Local != local || got.Remote != theirs {
		t.Errorf("Base diverged shows the local base at %s and the remote's at %s, want %s and %s", got.Local, got.Remote, local, theirs)
	}

	h.Git("reset", "-q", "--hard", "origin/main")
	r.Poll()
	h.WantState(id, domain.Done)
	if _, ok := baseDivergedItem(t, h); ok {
		t.Error("Base diverged still shows in Needs Me after the base was reset to the remote's")
	}
	h.WantCalls(1)
}

func TestUnderMergeAndPushAFailedFetchOnlyWarns(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	remote.Break()

	r := h.Start()
	r.WantOutput("warning: can't fetch origin")
	if _, ok := baseDivergedItem(t, h); ok {
		t.Error("a failed fetch showed Base diverged")
	}
	if tk := h.Ticket(id); tk.State != domain.Accepted {
		t.Errorf("%s is %s, want it left Accepted", id, tk.State)
	}

	remote.Restore()
	r.Poll()
	h.WantState(id, domain.Done)
}

func TestUnderMergeAndPushUncommittedChangesInTheWayMakeThePollWait(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.PushByHand(remote, "theirs", "main")
	mergeAndPush(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	inTheWay := filepath.Join(h.Repo, "theirs.txt")
	if err := os.WriteFile(inTheWay, []byte("mine, uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := h.Tip("main")

	r := h.Start()
	r.WantOutput("warning: can't fast-forward main to origin yet")
	if tk := h.Ticket(id); tk.State != domain.Accepted {
		t.Errorf("%s is %s, want it held Accepted", id, tk.State)
	}
	if got := h.Tip("main"); got != before {
		t.Errorf("main moved to %s, want it left at %s", got, before)
	}
	if _, ok := baseDivergedItem(t, h); ok {
		t.Error("uncommitted changes showed Base diverged")
	}

	if err := os.Remove(inTheWay); err != nil {
		t.Fatal(err)
	}
	r.Poll()
	h.WantState(id, domain.Done)
	if !h.IsAncestor(theirs, "main") {
		t.Errorf("%s is not in %s", theirs, "main")
	}
}

func TestUnderMergeWithoutPushTheFactoryNeverFetches(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
	remote := h.AddRemote("origin")
	h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.PushByHand(remote, "theirs", "main")
	// Some git versions record a tracking ref for a push to a configured
	// remote's URL; drop it so only a fetch by the Factory can leave one.
	for _, ref := range strings.Fields(h.Git("for-each-ref", "--format=%(refname)", "refs/remotes")) {
		h.Git("update-ref", "-d", ref)
	}
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingMerge, Remote: "origin"})
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")

	h.Start()
	h.WantState(id, domain.Done)
	if got := h.Git("branch", "-r"); got != "" {
		t.Errorf("remote-tracking branches %q, want none: nothing was fetched", got)
	}
}
