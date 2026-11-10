package frontdoor_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// CI status badges (code-central-259): the rows the Spec view, Needs Me
// and the Activity Dashboard read carry a Ticket's CI result, and leave
// it out for a Ticket with none.

// rowOf is id's row among rows, failing when it isn't there.
func rowOf(t *testing.T, rows []core.TicketRow, id string) core.TicketRow {
	t.Helper()
	for _, r := range rows {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no row %s in %+v", id, rows)
	return core.TicketRow{}
}

func TestFindTicketsAndSpecRowsCarryTheCIResult(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	reportRuns(t, srv, ciRun(1, "ci.yml", "abc123", store.CICompleted, store.CIFailure))
	ctx := context.Background()

	rows, err := srv.Client.FindTickets(ctx, "acme", core.FindTicketsInput{State: store.States})
	if err != nil {
		t.Fatal(err)
	}
	if got := rowOf(t, rows, "acme-3").CIResult; got != core.CIFailed {
		t.Fatalf("find_tickets row ci_result %q, want failed", got)
	}
	for _, r := range rows {
		if r.ID != "acme-3" && r.CIResult != "" {
			t.Fatalf("%s, never pushed, has ci_result %q, want none", r.ID, r.CIResult)
		}
	}

	spec, err := srv.Client.GetSpec(ctx, "acme", core.GetSpecInput{Spec: core.Ref(ticket3(t, srv).Spec.ID), Parts: []string{core.PartTickets}})
	if err != nil {
		t.Fatal(err)
	}
	if got := rowOf(t, spec.Tickets, "acme-3").CIResult; got != core.CIFailed {
		t.Fatalf("get_spec row ci_result %q, want failed", got)
	}
}

func TestNeedsMeRowsCarryTheCIResult(t *testing.T) {
	// Under Branch + Push an Accepted Ticket waits in Needs Me, To merge,
	// with its branch's pushed commit tracked.
	srv := newServer(t, func(t *testing.T, srv *codecentraltest.Server) {
		ciTrackingOn(t, srv)
		must(t, func() error {
			_, err := srv.Core.SetLandingMethod(core.WithActor(context.Background(), core.Human), "acme",
				core.SetLandingMethodInput{Method: store.LandingBranch, Push: true, Remote: "origin"})
			return err
		})
		acceptedTicket(t, srv)
	})
	row := func() core.NeedsMeRow {
		t.Helper()
		r, ok := needsMeRowOf(t, srv, "acme-3")
		if !ok {
			t.Fatal("acme-3 isn't in Needs Me")
		}
		return r
	}
	if got := row().CIResult; got != "" {
		t.Fatalf("before its push, ci_result %q, want none", got)
	}

	pushCommit(t, srv, "abc123")
	if got := row().CIResult; got != core.CIRunning {
		t.Fatalf("pushed, ci_result %q, want running", got)
	}
	reportRuns(t, srv, ciRun(1, "ci.yml", "abc123", store.CICompleted, store.CISuccess))
	if got := row().CIResult; got != core.CIPassed {
		t.Fatalf("its run passed, ci_result %q, want passed", got)
	}
}
