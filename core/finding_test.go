package core_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

func boolPtr(b bool) *bool { return &b }

func rowIDs(rows []api.TicketRow) []string {
	out := []string{}
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

func TestFindTicketsByTagsAndClaimable(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready, domain.Draft) // acme-2, acme-3, acme-4
	f.addBlocker(t, "acme-3", "acme-4")
	for _, in := range []api.SetTagsInput{
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
		in   api.FindTicketsInput
		want []string
	}{
		{"one tag, exact value", api.FindTicketsInput{Tags: map[string]string{"area": "web"}}, []string{"acme-2", "acme-3"}},
		{"every tag pair", api.FindTicketsInput{Tags: map[string]string{"area": "web", "size": "s"}}, []string{"acme-2"}},
		{"claimable", api.FindTicketsInput{Claimable: boolPtr(true)}, []string{"acme-2"}},
		{"not claimable", api.FindTicketsInput{Claimable: boolPtr(false)}, []string{"acme-3", "acme-4"}},
		{"combined", api.FindTicketsInput{Tags: map[string]string{"area": "web"}, Claimable: boolPtr(false)}, []string{"acme-3"}},
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
	rows, _ := f.core.FindTickets(ctx, "acme", api.FindTicketsInput{Title: "Tb"})
	if len(rows) != 1 || !rows[0].Blocked {
		t.Fatalf("row for blocked acme-3: %+v", rows)
	}
}

func TestGetSpecShowsItsProgress(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready, domain.Draft) // Spec acme-1; acme-2, acme-3, acme-4
	f.setState(t, 3, domain.Done)
	empty := f.spec(t, "acme", "Later") // acme-5

	v, err := f.core.GetSpec(ctx, "acme", api.GetSpecInput{Spec: "acme-1"})
	if err != nil {
		t.Fatal(err)
	}
	if v.ID != "acme-1" || v.Title != "Login" || *v.Body != "body\n" {
		t.Fatalf("spec %+v", v)
	}
	want := map[domain.State]int{domain.Ready: 1, domain.Done: 1, domain.Draft: 1}
	if !maps.Equal(v.StateSummary, want) || v.Finished {
		t.Fatalf("summary %v finished %v, want %v", v.StateSummary, v.Finished, want)
	}
	// Every Ticket of the Spec, finished ones too, in claim order.
	if got := rowIDs(v.Tickets); !slices.Equal(got, []string{"acme-2", "acme-3", "acme-4"}) {
		t.Fatalf("tickets %v", got)
	}

	v, err = f.core.GetSpec(ctx, "acme", api.GetSpecInput{Spec: api.Ref(empty.ID)})
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(v.StateSummary, map[domain.State]int{domain.Draft: 0}) || v.Finished || len(v.Tickets) != 0 || v.Tickets == nil {
		t.Fatalf("a Spec with no Tickets: summary %v finished %v tickets %v", v.StateSummary, v.Finished, v.Tickets)
	}

	_, err = f.core.GetSpec(ctx, "acme", api.GetSpecInput{Spec: "acme-2"})
	wantCode(t, err, api.NotFound, "")
}

func TestFindSpecsHidesFinishedSpecsUnlessAsked(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)                                                                   // Spec acme-1 "Login", acme-2
	f.spec(t, "acme", "Search")                                                               // acme-3, no Tickets: not finished
	f.spec(t, "acme", "Old login")                                                            // acme-4
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "4", Title: "Gone", State: domain.Ready}) // acme-5
	f.setState(t, 5, domain.Cancelled)
	if _, err := f.core.AddLabels(ctx, "acme", api.LabelsInput{Target: "acme-1", Labels: []string{"auth"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.SetTags(ctx, "acme", api.SetTagsInput{Target: "acme-3", Tags: map[string]string{"area": "web"}}); err != nil {
		t.Fatal(err)
	}

	ids := func(rows []api.SpecRow) []string {
		out := []string{}
		for _, r := range rows {
			out = append(out, r.ID)
		}
		return out
	}
	for _, c := range []struct {
		name string
		in   api.FindSpecsInput
		want []string
	}{
		{"unfinished only", api.FindSpecsInput{}, []string{"acme-1", "acme-3"}},
		{"finished too", api.FindSpecsInput{Finished: true}, []string{"acme-1", "acme-3", "acme-4"}},
		{"title", api.FindSpecsInput{Title: "LOGIN", Finished: true}, []string{"acme-1", "acme-4"}},
		{"labels", api.FindSpecsInput{Labels: []string{"auth"}}, []string{"acme-1"}},
		{"tags", api.FindSpecsInput{Tags: map[string]string{"area": "web"}}, []string{"acme-3"}},
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
	rows, _ := f.core.FindSpecs(ctx, "acme", api.FindSpecsInput{Finished: true})
	old := rows[2]
	if old.Title != "Old login" || !old.Finished || !maps.Equal(old.StateSummary, map[domain.State]int{domain.Cancelled: 1}) {
		t.Fatalf("row %+v", old)
	}
}

func TestTagFiltersNeedValidKeys(t *testing.T) {
	f := newFixture(t)
	f.work(t)
	_, err := f.core.FindTickets(ctx, "acme", api.FindTicketsInput{Tags: map[string]string{"Bad Key": "x"}})
	wantCode(t, err, api.Invalid, "tags")
	_, err = f.core.FindSpecs(ctx, "acme", api.FindSpecsInput{Tags: map[string]string{"Bad Key": "x"}})
	wantCode(t, err, api.Invalid, "tags")
}
