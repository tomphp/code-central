package core_test

import (
	"slices"
	"testing"

	"github.com/tomoram/code-central/api"
)

func blockedByOf(t *testing.T, f fixture, spec api.Ref, ticket string) ([]string, api.SpecDetail) {
	t.Helper()
	sp, err := f.core.GetSpec(ctx, "acme", api.GetSpecInput{Spec: spec})
	if err != nil {
		t.Fatalf("get_spec %s: %v", spec, err)
	}
	for _, row := range sp.Tickets {
		if row.ID == ticket {
			return row.BlockedBy, sp
		}
	}
	t.Fatalf("no row %s in %v", ticket, sp.Tickets)
	return nil, sp
}

func TestAPlainSpecsRowsCarryBlockersIncludingOnesOnAnotherSpec(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "One")                                                                          // acme-1
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "A"})                                 // acme-2
	f.spec(t, "acme", "Two")                                                                          // acme-3
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "3", Title: "B", BlockedBy: []api.Ref{"acme-2"}}) // acme-4
	got, sp := blockedByOf(t, f, "3", "acme-4")
	if !slices.Equal(got, []string{"acme-2"}) || sp.Tickets[0].NodeType != "" {
		t.Fatalf("blocked_by %v node_type %q", got, sp.Tickets[0].NodeType)
	}
	rows, err := f.core.FindTickets(ctx, "acme", api.FindTicketsInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if len(row.BlockedBy) != 0 {
			t.Fatalf("find_tickets rows on a plain Spec carry no blocked_by: %+v", row)
		}
	}
}

func TestARequirementsRowsCarryBlockers(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	var with int
	sp, err := f.core.GetSpec(ctx, "acme", api.GetSpecInput{Spec: "2"})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range sp.Tickets {
		if len(row.BlockedBy) > 0 {
			with++
		}
	}
	if with == 0 {
		t.Fatalf("no Requirement row carries blocked_by: %+v", sp.Tickets)
	}
}

func TestALongLivedSpecsRowsCarryNoBlockers(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "One")                                                                          // acme-1
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "A"})                                 // acme-2
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "B", BlockedBy: []api.Ref{"acme-2"}}) // acme-3
	f.longLived(t, api.SpecView{ID: "acme-1"})
	if got, _ := blockedByOf(t, f, "1", "acme-3"); len(got) != 0 {
		t.Fatalf("blocked_by %v", got)
	}
}

func TestAMapsRowsCarryBlockersAndNodeTypes(t *testing.T) {
	f := newFixture(t)
	f.aMap(t) // Map acme-1, Charting acme-2
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Next", Labels: []string{"wayfinder:task"}, BlockedBy: []api.Ref{"acme-2"}})
	got, sp := blockedByOf(t, f, "1", "acme-3")
	if !slices.Equal(got, []string{"acme-2"}) || sp.Tickets[1].NodeType != "task" {
		t.Fatalf("blocked_by %v node_type %q", got, sp.Tickets[1].NodeType)
	}
}
