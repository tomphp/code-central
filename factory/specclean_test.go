package factory_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

func off() *bool { v := false; return &v }

func TestTurningLandPerSpecOffLandsTheWaitingTicketsAsAGroupAndLaterOnesSingly(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	a, b := twoAccepted(t, h)
	release := holdTicketsStage(t, h)
	r := h.Start()
	r.WantOutput(b + ": merged into spec/" + h.SpecID())
	oldMain := h.Tip("main")

	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingMerge, LandPerSpec: off()})
	r.Poll()
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
	merge := h.Tip("main")
	if got := h.Log(merge)[0]; got != "Merge spec "+h.SpecID()+": Work" {
		t.Errorf("merge subject %q", got)
	}
	if h.Parents(merge)[0] != oldMain {
		t.Error("the Spec merge doesn't descend from the old base")
	}

	release()
	c, _ := accepted(h, "Later", "three.txt", "3\n")
	r.Poll()
	h.WantState(c, domain.Done)
	if got := h.Log("main")[0]; got != "Merge "+c+": Later" {
		t.Errorf("later Ticket landed with %q, want a plain merge", got)
	}
}

func TestTurningLandPerSpecOnHoldsOnlyLaterTickets(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.MakeRequirement()
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingMerge, LandPerSpec: off()})
	a, _ := accepted(h, "First", "one.txt", "1\n")
	r := h.Start()
	h.WantState(a, domain.Done)

	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingMerge, LandPerSpec: func() *bool { v := true; return &v }()})
	release := holdTicketsStage(t, h)
	b, _ := accepted(h, "Second", "two.txt", "2\n")
	r.Poll()
	r.WantOutput(b + ": merged into spec/" + h.SpecID())
	if h.Ticket(b).State != domain.Accepted {
		t.Error("the later Ticket landed before its Spec")
	}
	release()
	r.Poll()
	h.WantState(b, domain.Done)
}

// cancelDrafts cancels the Spec's Draft Stage Tickets, so only the
// implementation Tickets keep it unfinished.
func cancelDrafts(t *testing.T, h *factorytest.Rig) {
	t.Helper()
	rows, err := h.Server.Client.FindTickets(t.Context(), h.Project, api.FindTicketsInput{Spec: api.Ref(h.SpecID()), State: []domain.State{domain.Draft}})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if _, err := h.Human().Cancel(t.Context(), h.Project, api.CancelInput{Ticket: api.Ref(r.ID)}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestASpecBranchInTheBaseIsDeletedOnceEveryTicketIsDone(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	a, b := twoAccepted(t, h)
	cancelDrafts(t, h)
	r := h.Start()
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
	r.Poll()
	r.WantOutput(h.SpecID() + ": deleted spec/" + h.SpecID() + ", which is in main")
	if out := h.SpecBranches(); len(out) != 0 {
		t.Errorf("Spec Branch still there: %q", out)
	}
	if !h.Exists(a) {
		t.Error("a Ticket branch was deleted")
	}
}

func TestAnUnmergedSpecBranchIsKeptWithOneLine(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.MakeRequirement()
	a, _ := accepted(h, "First", "one.txt", "1\n")
	release := holdTicketsStage(t, h)
	r := h.Start()
	r.WantOutput(a + ": merged into spec/" + h.SpecID())
	if _, err := h.Human().Cancel(t.Context(), h.Project, api.CancelInput{Ticket: api.Ref(a)}); err != nil {
		t.Fatal(err)
	}
	release()
	cancelDrafts(t, h)
	r.Poll()
	r.WantOutput(h.SpecID() + ": every Ticket is finished but spec/" + h.SpecID() + " isn't in main: kept")
	r.Poll()
	if len(h.SpecBranches()) == 0 {
		t.Error("the unmerged Spec Branch was deleted")
	}
	if n := strings.Count(r.Stdout(), "isn't in main: kept"); n != 1 {
		t.Errorf("line printed %d times", n)
	}
}

func TestAFinishedTaggedSpecsBranchIsDeletedOnceInTheBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	taggedRequirement(t, h)
	a, b := twoAccepted(t, h)
	cancelDrafts(t, h)
	r := h.Start()
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
	r.Poll()
	r.WantOutput(h.SpecID() + ": deleted " + taggedBranch + ", which is in main")
	wantNoBranch(t, h, taggedBranch)
}

func TestAnUnfinishedTaggedSpecsBranchIsKept(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	taggedRequirement(t, h)
	a, _ := accepted(h, "First", "one.txt", "1\n")
	release := holdTicketsStage(t, h)
	defer release()
	r := h.Start()
	r.WantOutput(a + ": merged into " + taggedBranch)
	r.Poll()
	if !h.Exists(taggedBranch) {
		t.Error("the unfinished Spec's branch was deleted")
	}
}
