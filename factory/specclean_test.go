package factory_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

func off() *bool { v := false; return &v }

func TestTurningLandPerSpecOffLandsTheWaitingTicketsAsAGroupAndLaterOnesSingly(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	a, b := twoAccepted(t, h)
	release := holdTicketsStage(t, h)
	r := h.Start()
	r.WaitForOutput(b + ": merged into spec/" + h.SpecID())
	oldMain := h.Git("rev-parse", "main")

	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingMerge, LandPerSpec: off()})
	r.Poll()
	h.WaitForState(a, store.Done)
	h.WaitForState(b, store.Done)
	merge := h.Git("rev-parse", "main")
	if got := h.Git("log", "-1", "--format=%s", merge); got != "Merge spec "+h.SpecID()+": Work" {
		t.Errorf("merge subject %q", got)
	}
	if h.Git("merge-base", oldMain, merge) != oldMain {
		t.Error("the Spec merge doesn't descend from the old base")
	}

	release()
	c, _ := accepted(h, "Later", "three.txt", "3\n")
	r.Poll()
	h.WaitForState(c, store.Done)
	if got := h.Git("log", "-1", "--format=%s", "main"); got != "Merge "+c+": Later" {
		t.Errorf("later Ticket landed with %q, want a plain merge", got)
	}
}

func TestTurningLandPerSpecOnHoldsOnlyLaterTickets(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingMerge, LandPerSpec: off()})
	a, _ := accepted(h, "First", "one.txt", "1\n")
	r := h.Start()
	h.WaitForState(a, store.Done)

	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingMerge, LandPerSpec: func() *bool { v := true; return &v }()})
	release := holdTicketsStage(t, h)
	b, _ := accepted(h, "Second", "two.txt", "2\n")
	r.Poll()
	r.WaitForOutput(b + ": merged into spec/" + h.SpecID())
	if h.Ticket(b).State != store.Accepted {
		t.Error("the later Ticket landed before its Spec")
	}
	release()
	r.Poll()
	h.WaitForState(b, store.Done)
}

// cancelDrafts cancels the Spec's Draft Stage Tickets, so only the
// implementation Tickets keep it unfinished.
func cancelDrafts(t *testing.T, h *factorytest.Harness) {
	t.Helper()
	rows, err := h.Server.Client.FindTickets(t.Context(), h.Project, core.FindTicketsInput{Spec: core.Ref(h.SpecID()), State: []store.State{store.Draft}})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if _, err := h.Human().Cancel(t.Context(), h.Project, core.CancelInput{Ticket: core.Ref(r.ID)}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestASpecBranchInTheBaseIsDeletedOnceEveryTicketIsDone(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	a, b := twoAccepted(t, h)
	cancelDrafts(t, h)
	r := h.Start()
	h.WaitForState(a, store.Done)
	h.WaitForState(b, store.Done)
	r.Poll()
	r.WaitForOutput(h.SpecID() + ": deleted spec/" + h.SpecID() + ", which is in main")
	if out := h.Git("branch", "--list", "spec/*"); out != "" {
		t.Errorf("Spec Branch still there: %q", out)
	}
	if out := h.Git("branch", "--list", a); out == "" {
		t.Error("a Ticket branch was deleted")
	}
}

func TestAnUnmergedSpecBranchIsKeptWithOneLine(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	a, _ := accepted(h, "First", "one.txt", "1\n")
	release := holdTicketsStage(t, h)
	r := h.Start()
	r.WaitForOutput(a + ": merged into spec/" + h.SpecID())
	if _, err := h.Human().Cancel(t.Context(), h.Project, core.CancelInput{Ticket: core.Ref(a)}); err != nil {
		t.Fatal(err)
	}
	release()
	cancelDrafts(t, h)
	r.Poll()
	r.WaitForOutput(h.SpecID() + ": every Ticket is finished but spec/" + h.SpecID() + " isn't in main: kept")
	r.Poll()
	if out := h.Git("branch", "--list", "spec/*"); out == "" {
		t.Error("the unmerged Spec Branch was deleted")
	}
	if n := strings.Count(r.Stdout(), "isn't in main: kept"); n != 1 {
		t.Errorf("line printed %d times", n)
	}
}
