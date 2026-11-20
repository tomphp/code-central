package factory

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/api"
)

// pushWorld is a landWorld whose Home Repository pushes to origin, and
// pushes the Spec Branch spec/acme-1 there, as the Spec's ready Spec
// Branch under Pull Request, with no Tickets waiting.
type pushWorld struct {
	*landWorld
}

const pushedSpecBranch = "spec/acme-1"

func newPushWorld(t *testing.T) *pushWorld {
	w := newLandWorld(t)
	w.repo.addRemote("origin")
	w.l.repos.home.remote = "origin"
	return &pushWorld{landWorld: w}
}

// push pushes the Spec Branch at tip, forced under rebase, given the
// Repositories its Tickets' pushed commits name.
func (w *pushWorld) push(tip string, rebase bool, pushedIn ...string) {
	w.repo.setBranch(pushedSpecBranch, tip)
	if _, err := w.repo.Fetch(context.Background(), "origin"); err != nil {
		w.t.Fatal(err)
	}
	spec := api.SpecRef{ID: "acme-1", Branch: pushedSpecBranch}
	w.l.pushSpecBranch(context.Background(), spec, pushedIn, api.LandingPolicies{}, specPlan{r: w.l.repos.home, tip: tip, rebase: rebase})
}

func TestUnderRebaseASpecBranchThisRunPushedIsForcePushedThoughItsReportFailed(t *testing.T) {
	t.Parallel()
	w := newPushWorld(t)
	first := w.repo.commitOn("first", "First")
	w.push(first, true)

	// No Ticket has the push reported, as when every report failed; the
	// Spec Branch is then rebased away from what was pushed.
	rebased := w.repo.commitOn("rebased", "Rebased")
	w.push(rebased, true)
	if got := w.repo.remoteTip("origin", pushedSpecBranch); got != rebased {
		t.Errorf("the remote's %s is at %q, want the rebased %s over this run's own push %s:\n%s", pushedSpecBranch, got, rebased, first, w.out)
	}
}

func TestASpecBranchIsPushedOverOneATicketsPushedCommitNamesButNotOverAnother(t *testing.T) {
	t.Parallel()
	w := newPushWorld(t)
	hand := w.repo.commitOn("hand", "By hand")
	w.repo.setRemoteBranch("origin", pushedSpecBranch, hand)
	tip := w.repo.commitOn("tip", "Tip")

	w.push(tip, true)
	if got := w.repo.remoteTip("origin", pushedSpecBranch); got != hand || w.repo.pushes.Load() != 0 {
		t.Fatalf("the remote's %s is at %q after %d pushes, want the hand-made %s untouched", pushedSpecBranch, got, w.repo.pushes.Load(), hand)
	}

	// Pushed by the Factory, as a Ticket's pushed commit says: its own.
	w.push(tip, true, "acme")
	if got := w.repo.remoteTip("origin", pushedSpecBranch); got != tip {
		t.Errorf("the remote's %s is at %q, want the Spec Branch %s over the Factory's own push", pushedSpecBranch, got, tip)
	}
}

func TestASpecBranchDeletedFromTheRemoteSinceTheFetchIsPushedAndItsStaleTrackingRefForgotten(t *testing.T) {
	t.Parallel()
	w := newPushWorld(t)
	hand := w.repo.commitOn("hand", "By hand")
	w.repo.setRemoteBranch("origin", pushedSpecBranch, hand)
	tip := w.repo.commitOn("tip", "Tip")
	w.push(tip, false)

	// The fetch doesn't prune, so the tracking ref stays.
	w.repo.deleteRemoteBranch("origin", pushedSpecBranch)
	w.l.pushSpecBranch(context.Background(), api.SpecRef{ID: "acme-1", Branch: pushedSpecBranch}, nil, api.LandingPolicies{}, specPlan{r: w.l.repos.home, tip: tip, rebase: true})
	if got := w.repo.remoteTip("origin", pushedSpecBranch); got != tip {
		t.Errorf("the remote's %s is at %q, want the Spec Branch %s once the hand-made one is gone:\n%s", pushedSpecBranch, got, tip, w.out)
	}
}

func TestASpecBranchIsntPushedWhileTheRemoteCantBeAskedForIt(t *testing.T) {
	t.Parallel()
	w := newPushWorld(t)
	hand := w.repo.commitOn("hand", "By hand")
	w.repo.setRemoteBranch("origin", pushedSpecBranch, hand)
	tip := w.repo.commitOn("tip", "Tip")
	w.push(tip, false) // fetches while the remote works
	w.repo.setBroken("origin", true)

	w.l.pushSpecBranch(context.Background(), api.SpecRef{ID: "acme-1", Branch: pushedSpecBranch}, nil, api.LandingPolicies{}, specPlan{r: w.l.repos.home, tip: tip, rebase: true})
	if w.repo.pushes.Load() != 0 {
		t.Errorf("the Spec Branch was pushed %d times, want none while origin can't be asked", w.repo.pushes.Load())
	}
	if w.lines("warning: can't ask origin for "+pushedSpecBranch) != 1 {
		t.Errorf("want a warning that origin can't be asked, got %q", w.out)
	}
}
