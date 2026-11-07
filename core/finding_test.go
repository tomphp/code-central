package core_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

func boolPtr(b bool) *bool { return &b }

func rowIDs(rows []core.TicketRow) []string {
	out := []string{}
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

func TestFindTicketsByTagsAndClaimable(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready, store.Draft) // acme-2, acme-3, acme-4
	f.addBlocker(t, "acme-3", "acme-4")
	for _, in := range []core.SetTagsInput{
		{Target: "acme-2", Tags: map[string]string{"area": "web", "size": "s"}},
		{Target: "acme-3", Tags: map[string]string{"area": "web"}},
		{Target: "acme-4", Tags: map[string]string{"area": "Web"}},
	} {
		if _, err := f.core.SetTags(ctx, "acme", in); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		name string
		in   core.FindTicketsInput
		want []string
	}{
		{"one tag, exact value", core.FindTicketsInput{Tags: map[string]string{"area": "web"}}, []string{"acme-2", "acme-3"}},
		{"every tag pair", core.FindTicketsInput{Tags: map[string]string{"area": "web", "size": "s"}}, []string{"acme-2"}},
		{"claimable", core.FindTicketsInput{Claimable: boolPtr(true)}, []string{"acme-2"}},
		{"not claimable", core.FindTicketsInput{Claimable: boolPtr(false)}, []string{"acme-3", "acme-4"}},
		{"combined", core.FindTicketsInput{Tags: map[string]string{"area": "web"}, Claimable: boolPtr(false)}, []string{"acme-3"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			rows, err := f.core.FindTickets(ctx, "acme", c.in)
			if err != nil {
				t.Fatal(err)
			}
			if got := rowIDs(rows); !slices.Equal(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
	rows, _ := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{Title: "Tb"})
	if len(rows) != 1 || !rows[0].Blocked {
		t.Fatalf("row for blocked acme-3: %+v", rows)
	}
}

func TestGetSpecShowsItsProgress(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready, store.Draft) // Spec acme-1; acme-2, acme-3, acme-4
	f.setState(t, 3, store.Done)
	empty := f.spec(t, "acme", "Later") // acme-5

	v, err := f.core.GetSpec(ctx, "acme", "acme-1")
	if err != nil {
		t.Fatal(err)
	}
	if v.ID != "acme-1" || v.Title != "Login" || v.Body != "body\n" {
		t.Fatalf("spec %+v", v.SpecView)
	}
	want := map[store.State]int{store.Ready: 1, store.Done: 1, store.Draft: 1}
	if !maps.Equal(v.StateSummary, want) || v.Finished {
		t.Fatalf("summary %v finished %v, want %v", v.StateSummary, v.Finished, want)
	}
	// Every Ticket of the Spec, finished ones too, in claim order.
	if got := rowIDs(v.Tickets); !slices.Equal(got, []string{"acme-2", "acme-3", "acme-4"}) {
		t.Fatalf("tickets %v", got)
	}

	v, err = f.core.GetSpec(ctx, "acme", core.Ref(empty.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(v.StateSummary, map[store.State]int{store.Draft: 0}) || v.Finished || len(v.Tickets) != 0 || v.Tickets == nil {
		t.Fatalf("a Spec with no Tickets: summary %v finished %v tickets %v", v.StateSummary, v.Finished, v.Tickets)
	}

	_, err = f.core.GetSpec(ctx, "acme", "acme-2")
	wantCode(t, err, core.NotFound, "")
}

func TestFindSpecsHidesFinishedSpecsUnlessAsked(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)                                                                    // Spec acme-1 "Login", acme-2
	f.spec(t, "acme", "Search")                                                               // acme-3, no Tickets: not finished
	f.spec(t, "acme", "Old login")                                                            // acme-4
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "4", Title: "Gone", State: store.Ready}) // acme-5
	f.setState(t, 5, store.Cancelled)
	if _, err := f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: "acme-1", Labels: []string{"auth"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.SetTags(ctx, "acme", core.SetTagsInput{Target: "acme-3", Tags: map[string]string{"area": "web"}}); err != nil {
		t.Fatal(err)
	}

	ids := func(rows []core.SpecRow) []string {
		out := []string{}
		for _, r := range rows {
			out = append(out, r.ID)
		}
		return out
	}
	for _, c := range []struct {
		name string
		in   core.FindSpecsInput
		want []string
	}{
		{"unfinished only", core.FindSpecsInput{}, []string{"acme-1", "acme-3"}},
		{"finished too", core.FindSpecsInput{Finished: true}, []string{"acme-1", "acme-3", "acme-4"}},
		{"title", core.FindSpecsInput{Title: "LOGIN", Finished: true}, []string{"acme-1", "acme-4"}},
		{"labels", core.FindSpecsInput{Labels: []string{"auth"}}, []string{"acme-1"}},
		{"tags", core.FindSpecsInput{Tags: map[string]string{"area": "web"}}, []string{"acme-3"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			rows, err := f.core.FindSpecs(ctx, "acme", c.in)
			if err != nil {
				t.Fatal(err)
			}
			if got := ids(rows); !slices.Equal(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
	rows, _ := f.core.FindSpecs(ctx, "acme", core.FindSpecsInput{Finished: true})
	old := rows[2]
	if old.Title != "Old login" || !old.Finished || !maps.Equal(old.StateSummary, map[store.State]int{store.Cancelled: 1}) {
		t.Fatalf("row %+v", old)
	}
}

func TestTagFiltersNeedValidKeys(t *testing.T) {
	f := newFixture(t)
	f.work(t)
	_, err := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{Tags: map[string]string{"Bad Key": "x"}})
	wantCode(t, err, core.Invalid, "tags")
	_, err = f.core.FindSpecs(ctx, "acme", core.FindSpecsInput{Tags: map[string]string{"Bad Key": "x"}})
	wantCode(t, err, core.Invalid, "tags")
}
