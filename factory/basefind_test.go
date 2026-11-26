package factory_test

import (
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

func setBase(h *factorytest.Rig, base string) {
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingMerge, Base: &base})
}

// baseMissingItem is the Project's Base missing item in Needs Me.
func baseMissingItem(t *testing.T, h *factorytest.Rig) (api.NeedsMeRow, bool) {
	t.Helper()
	if rows := h.NeedsMeItems(api.ReasonBaseMissing); len(rows) > 0 {
		return rows[0], true
	}
	return api.NeedsMeRow{}, false
}

func TestABaseOnlyOnTheRemoteIsCreatedLocally(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	h.Publish(remote, h.Tip("main"), "release")
	want := h.Tip("main")
	setBase(h, "release")
	id := h.ReadyTicket("On release")

	r := h.Start()
	r.WantOutput("base: created release at")
	h.WantState(id, domain.InReview)
	if got := h.Tip("release"); got != want {
		t.Errorf("release is at %s, want the remote's %s", got, want)
	}
}

func TestAMissingBaseHoldsClaimsAndLandingUntilItIsCreated(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.AddRemote("origin")
	setBase(h, "release")
	landing, _ := accepted(h, "Waits to land", "feature.txt", "feature\n")
	ready := h.ReadyTicket("Waits to start")

	r := h.Start()
	r.WantOutput("the base, release, exists neither here nor on origin")
	r.Poll()
	if row, ok := baseMissingItem(t, h); !ok || row.BaseMissing == nil || row.BaseMissing.Base != "release" {
		t.Errorf("Needs Me has no Base missing item for release (found one: %v, its Base %+v)", ok, row.BaseMissing)
	}
	r.Poll()
	if s := h.Ticket(ready).State; s != domain.Ready {
		t.Errorf("%s is %s, want it not claimed", ready, s)
	}
	if s := h.Ticket(landing).State; s != domain.Accepted {
		t.Errorf("%s is %s, want it not landed", landing, s)
	}

	h.NewBranch("release", "main")
	r.Poll()
	r.WantOutput("base: found release again")
	h.WantState(ready, domain.InReview)
	h.WantState(landing, domain.Done)
	if _, ok := baseMissingItem(t, h); ok {
		t.Errorf("Needs Me still has the Base missing item once release exists")
	}
}

func TestABaseWithoutMCPJSONHoldsNewBranchesUntilItIsCommitted(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.EmptyBranch("release")

	r := h.Start()
	setBase(h, "release")
	ready := h.ReadyTicket("Waits for .mcp.json")
	r.Poll()
	r.WantOutput(".mcp.json isn't committed on release")
	r.Poll()
	if s := h.Ticket(ready).State; s != domain.Ready {
		t.Errorf("%s is %s, want it not claimed", ready, s)
	}

	h.MoveBranch("release", "main")
	r.Poll()
	h.WantState(ready, domain.InReview)
}
