package factory_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

func setBase(h *factorytest.Harness, base string) {
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingMerge, Base: &base})
}

// baseMissingItem is the Project's Base missing item in Needs Me.
func baseMissingItem(t *testing.T, h *factorytest.Harness) (core.NeedsMeRow, bool) {
	t.Helper()
	v, err := h.Human().NeedsMe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range v.Groups {
		if g.Reason == core.ReasonBaseMissing {
			for _, row := range g.Tickets {
				if row.Project == h.Project {
					return row, true
				}
			}
		}
	}
	return core.NeedsMeRow{}, false
}

func TestABaseOnlyOnTheRemoteIsCreatedLocally(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	h.Git("push", "-q", remote.Path, "main:refs/heads/release")
	want := h.Git("rev-parse", "main")
	setBase(h, "release")
	id := h.ReadyTicket("On release")

	r := h.Start()
	r.WaitForOutput("base: created release at")
	h.WaitForState(id, store.InReview)
	if got := h.Git("rev-parse", "refs/heads/release"); got != want {
		t.Errorf("release is at %s, want the remote's %s", got, want)
	}
}

func TestAMissingBaseHoldsClaimsAndLandingUntilItIsCreated(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	setBase(h, "release")
	landing, _ := accepted(h, "Waits to land", "feature.txt", "feature\n")
	ready := h.ReadyTicket("Waits to start")

	r := h.Start()
	r.WaitForOutput("the base, release, exists neither here nor on origin")
	r.Poll()
	h.WaitFor("Base missing shows in Needs Me", func() bool {
		row, ok := baseMissingItem(t, h)
		return ok && row.BaseMissing != nil && row.BaseMissing.Base == "release"
	})
	r.Poll()
	if s := h.Ticket(ready).State; s != store.Ready {
		t.Errorf("%s is %s, want it not claimed", ready, s)
	}
	if s := h.Ticket(landing).State; s != store.Accepted {
		t.Errorf("%s is %s, want it not landed", landing, s)
	}

	h.Git("branch", "release", "main")
	r.Poll()
	r.WaitForOutput("base: found release again")
	h.WaitForState(ready, store.InReview)
	h.WaitForState(landing, store.Done)
	h.WaitFor("Base missing clears", func() bool { _, ok := baseMissingItem(t, h); return !ok })
}

func TestABaseWithoutMCPJSONHoldsNewBranchesUntilItIsCommitted(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	empty := h.Git("commit-tree", "-m", "empty", "4b825dc642cb6eb9a060e54bf8d69288fbee4904")
	h.Git("branch", "release", empty)

	r := h.Start()
	setBase(h, "release")
	ready := h.ReadyTicket("Waits for .mcp.json")
	r.Poll()
	r.WaitForOutput(".mcp.json isn't committed on release")
	r.Poll()
	if s := h.Ticket(ready).State; s != store.Ready {
		t.Errorf("%s is %s, want it not claimed", ready, s)
	}

	h.Git("branch", "-f", "release", "main")
	r.Poll()
	h.WaitForState(ready, store.InReview)
}
