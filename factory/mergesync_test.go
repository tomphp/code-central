package factory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// Under Merge + Push the Factory fetches the Push remote and brings the
// local base level with it before it lands or cuts a branch
// (code-central-428): behind, it fast-forwards; level or ahead, it is
// current; both moved, it reports Base diverged and waits.

func TestUnderMergeAndPushARemoteThatIsAheadIsFastForwardedBeforeTheTicketLands(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	// Pushed by path, so the repo's origin/main doesn't know of it.
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")
	mergeAndPush(h, "origin")
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	r.WaitForOutput("main: fast-forwarded to origin/main")
	tk := h.WaitForState(id, store.Done)
	merge := h.Git("rev-parse", "main")
	if got := h.Git("log", "-1", "--format=%P", merge); got != theirs+" "+commit {
		t.Errorf("main's tip has parents %q, want the remote's %s then %s", got, theirs, commit)
	}
	if got := remote.Git("rev-parse", "main"); got != merge {
		t.Errorf("the remote's main is at %q, want the merge %s on top of theirs", got, merge)
	}
	if m := tk.Moves[len(tk.Moves)-1]; m.Commit != merge {
		t.Errorf("landed with %q, want the merge %s", m.Commit, merge)
	}
}

func TestUnderMergeAndPushWithCITrackingARemoteThatIsAheadIsFastForwarded(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")
	mergeAndPush(h, "origin")
	trackCI(t, h)
	ready := h.ReadyTicket("Starts from the remote's base")

	h.Start()
	h.WaitFor("the turn", func() bool { return len(h.Calls()) == 1 })
	if got := h.Git("rev-parse", "main"); got != theirs {
		t.Errorf("the local base is at %s, want it fast-forwarded to %s", got, theirs)
	}
	h.Git("merge-base", "--is-ancestor", theirs, ready)
}

func TestUnderMergeAndPushANewBranchIsCutFromTheFastForwardedBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	h.Git("push", "-q", remote.Path, "main:refs/heads/main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")
	ready := h.ReadyTicket("Starts from the remote's base")

	h.Start()
	h.WaitFor("the turn", func() bool { return len(h.Calls()) == 1 })
	if got := h.Git("rev-parse", "main"); got != theirs {
		t.Errorf("the local base is at %s, want it fast-forwarded to %s", got, theirs)
	}
	if h.Calls()[0].Ticket != ready {
		t.Fatalf("calls %+v, want %s claimed", h.Calls(), ready)
	}
	h.Git("merge-base", "--is-ancestor", theirs, ready)
}

func TestUnderMergeAndPushALocalBaseAheadOfTheRemoteIsCurrent(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	h.Git("push", "-q", remote.Path, "main:refs/heads/main")
	mine := h.CommitOn("main", "mine.txt", "mine\n")
	mergeAndPush(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	h.WaitForState(id, store.Done)
	if _, ok := baseDivergedItem(t, h); ok {
		t.Error("an unpushed local commit showed Base diverged")
	}
	remote.Git("merge-base", "--is-ancestor", mine, "main")
	if out := r.Stdout() + r.Stderr(); strings.Contains(out, "diverged") {
		t.Errorf("the Factory reported a divergence:\n%s", out)
	}
}

func TestUnderMergeAndPushADivergedBaseStopsLandingAndClaiming(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	h.Git("push", "-q", remote.Path, "main:refs/heads/main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")
	h.CommitOn("main", "mine.txt", "mine\n")
	ready := h.ReadyTicket("Waits for the base")

	r := h.Start()
	r.WaitForOutput("has diverged from the remote's")
	if tk := h.Ticket(id); tk.State != store.Accepted {
		t.Errorf("%s is %s, want it held Accepted", id, tk.State)
	}
	if tk := h.Ticket(ready); tk.State != store.Ready {
		t.Errorf("%s is %s, want it not claimed", ready, tk.State)
	}
	if got := remote.Git("rev-parse", "main"); got != theirs {
		t.Errorf("the remote's main is at %q, want it left at %s", got, theirs)
	}
	local := h.Git("rev-parse", "main")
	r.Poll()
	h.WaitFor("Base diverged shows in Needs Me", func() bool {
		row, ok := baseDivergedItem(t, h)
		return ok && row.BaseDiverged.Local == local && row.BaseDiverged.Remote == theirs
	})

	h.Git("reset", "-q", "--hard", "origin/main")
	r.Poll()
	h.WaitForState(id, store.Done)
	h.WaitFor("Base diverged clears", func() bool { _, ok := baseDivergedItem(t, h); return !ok })
	h.WaitFor("the turn", func() bool { return len(h.Calls()) == 1 })
}

func TestUnderMergeAndPushAFailedFetchOnlyWarns(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	remote.Break()

	r := h.Start()
	r.WaitForOutput("warning: can't fetch origin")
	if _, ok := baseDivergedItem(t, h); ok {
		t.Error("a failed fetch showed Base diverged")
	}
	if tk := h.Ticket(id); tk.State != store.Accepted {
		t.Errorf("%s is %s, want it left Accepted", id, tk.State)
	}

	remote.Restore()
	r.Poll()
	h.WaitForState(id, store.Done)
}

func TestUnderMergeAndPushUncommittedChangesInTheWayMakeThePollWait(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")
	mergeAndPush(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	inTheWay := filepath.Join(h.Repo, "theirs.txt")
	if err := os.WriteFile(inTheWay, []byte("mine, uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := h.Git("rev-parse", "main")

	r := h.Start()
	r.WaitForOutput("warning: can't fast-forward main to origin yet")
	if tk := h.Ticket(id); tk.State != store.Accepted {
		t.Errorf("%s is %s, want it held Accepted", id, tk.State)
	}
	if got := h.Git("rev-parse", "main"); got != before {
		t.Errorf("main moved to %s, want it left at %s", got, before)
	}
	if _, ok := baseDivergedItem(t, h); ok {
		t.Error("uncommitted changes showed Base diverged")
	}

	if err := os.Remove(inTheWay); err != nil {
		t.Fatal(err)
	}
	r.Poll()
	h.WaitForState(id, store.Done)
	h.Git("merge-base", "--is-ancestor", theirs, "main")
}

func TestUnderMergeWithoutPushTheFactoryNeverFetches(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingMerge, Remote: "origin"})
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")

	h.Start()
	h.WaitForState(id, store.Done)
	if got := h.Git("branch", "-r"); got != "" {
		t.Errorf("remote-tracking branches %q, want none: nothing was fetched", got)
	}
}
