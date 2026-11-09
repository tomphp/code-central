package core_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

const offlineMode = "Offline mode\n\nThe app should work on a plane.\n"

// mapBody is offlineMode as submit_map writes it: the human's text, then
// the Map's empty sections.
const mapBody = offlineMode + "\n## Destination\n\n## Notes\n\n## Decisions so far\n\n## Not yet specified\n\n## Out of scope\n"

func TestSubmittingAMapCreatesItsSpecAndOneReadyChartingTicket(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Existing") // acme-1, so the Map starts at acme-2
	res, err := f.core.SubmitMap(ctx, "acme", core.SubmitMapInput{Text: offlineMode})
	if err != nil {
		t.Fatalf("submit_map: %v", err)
	}
	if res.Spec.ID != "acme-2" || res.Charting.ID != "acme-3" {
		t.Fatalf("result %+v", res)
	}
	sp, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-2"})
	if err != nil {
		t.Fatalf("get_spec: %v", err)
	}
	if sp.Title != "Offline mode" || *sp.Body != mapBody || !slices.Equal(sp.Labels, []string{core.MapLabel}) {
		t.Fatalf("spec %q %q %v", sp.Title, *sp.Body, sp.Labels)
	}
	tk := f.get(t, "acme-3")
	if tk.Title != "Charting: Offline mode" || tk.State != store.Ready || tk.Spec.ID != "acme-2" ||
		!slices.Equal(tk.Labels, []string{"wayfinder:chart"}) || !tk.Claimable {
		t.Fatalf("charting %+v", tk)
	}
	if !strings.Contains(tk.Description, "acme-2") {
		t.Fatalf("the Charting Ticket's description should point at the Map: %q", tk.Description)
	}
	if len(tk.Moves) != 1 || tk.Moves[0].Actor != "human" || tk.Moves[0].Via != "submit_map" {
		t.Fatalf("a Map is written as the human whoever submits it: %+v", tk.Moves)
	}
}

func TestTheMapsSectionsAreLevelTwoHeadingsInOrder(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	if _, err := f.core.SubmitMap(ctx, "acme", core.SubmitMapInput{Text: "# Offline mode"}); err != nil {
		t.Fatalf("submit_map: %v", err)
	}
	sp, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-1", Parts: []string{"body"}})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, h := range sp.Outline {
		if h.Level == 2 {
			got = append(got, h.Text)
		}
	}
	want := []string{"Destination", "Notes", "Decisions so far", "Not yet specified", "Out of scope"}
	if !slices.Equal(got, want) || sp.Title != "Offline mode" {
		t.Fatalf("headings %q, title %q", got, sp.Title)
	}
}

func TestAMapNeedsTextAndWritesNothingWithout(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	for _, text := range []string{"", "  \n\t\n"} {
		_, err := f.core.SubmitMap(ctx, "acme", core.SubmitMapInput{Text: text})
		wantCode(t, err, core.Invalid, "text")
	}
	f.store.View(func(r store.Reader) error {
		if len(r.Specs("acme")) != 0 || len(r.Tickets("acme")) != 0 {
			t.Fatal("a rejected Map wrote something")
		}
		return nil
	})
}

func TestSubmittingAMapAcknowledgesItsSpecAndChartingTicket(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	res, err := f.core.SubmitMap(ctx, "acme", core.SubmitMapInput{Text: offlineMode})
	if err != nil {
		t.Fatalf("submit_map: %v", err)
	}
	wantAcknowledgement(t, res.Spec, "acme-1", t0, []string{"title", "labels", "body"}, 0)
	wantTicketAcknowledgement(t, res.Charting, "acme-2", t0, store.Ready,
		[]string{"title", "state", "spec", "labels", "description", "moves", "claimable"}, 0)

	full, err := f.core.SubmitMap(ctx, "acme", core.SubmitMapInput{Text: offlineMode, Full: true})
	if err != nil || full.Spec.Record.Body != mapBody || full.Charting.Record.Title != "Charting: Offline mode" {
		t.Fatalf("full %+v, %v", full, err)
	}
}

// mapWith is a Project acme with the Map acme-1, whose Charting Ticket is
// acme-2, and these Tickets on it, created in order from acme-3.
func (f fixture) mapWith(t *testing.T, tickets ...core.CreateTicketInput) {
	t.Helper()
	f.project(t, "acme")
	if _, err := f.core.SubmitMap(ctx, "acme", core.SubmitMapInput{Text: offlineMode}); err != nil {
		t.Fatalf("submit_map: %v", err)
	}
	for _, in := range tickets {
		in.Spec = "acme-1"
		f.ticket(t, "acme", in)
	}
	f.clock.Advance(time.Minute)
}

func (f fixture) mapOf(t *testing.T, spec core.Ref) *core.MapView {
	t.Helper()
	sp, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: spec})
	if err != nil {
		t.Fatalf("get_spec %s: %v", spec, err)
	}
	return sp.Map
}

func TestOnlyASpecLabelledAsAMapCarriesAMapBlock(t *testing.T) {
	f := newFixture(t)
	f.mapWith(t)
	f.spec(t, "acme", "Login") // acme-3
	if m := f.mapOf(t, "acme-3"); m != nil {
		t.Fatalf("a plain Spec has no Map block: %+v", m)
	}
	m := f.mapOf(t, "acme-1")
	if m == nil || *m != (core.MapView{Decided: 0, Total: 1, NeedsYou: 0, CanWriteSpec: false}) {
		t.Fatalf("a new Map: %+v", m)
	}
	rows, err := f.core.FindSpecs(ctx, "acme", core.FindSpecsInput{})
	if err != nil || len(rows) != 2 || rows[0].Map == nil || rows[1].Map != nil {
		t.Fatalf("find_specs rows %+v, %v", rows, err)
	}
	if *rows[0].Map != *m {
		t.Fatalf("the row's Map block %+v, get_spec's %+v", rows[0].Map, m)
	}
}

func TestTheMapBlockCountsItsDecisionsAndWhatNeedsTheHuman(t *testing.T) {
	f := newFixture(t)
	f.mapWith(t,
		core.CreateTicketInput{Title: "Which store", Labels: []string{"wayfinder:grilling"}},    // acme-3
		core.CreateTicketInput{Title: "Sync libraries", Labels: []string{"wayfinder:research"}}, // acme-4
		core.CreateTicketInput{Title: "Ask legal", Labels: []string{"wayfinder:task"}},          // acme-5
		core.CreateTicketInput{Title: "Dropped", Labels: []string{"wayfinder:grilling"}},        // acme-6
		core.CreateTicketInput{Title: "Try a cache", Labels: []string{"prototype"}},             // acme-7
		core.CreateTicketInput{Title: "Unlabelled"},                                             // acme-8
	)
	f.setState(t, 2, store.Done)
	f.setState(t, 3, store.Done)
	f.setState(t, 4, store.InReview)
	f.setState(t, 6, store.Cancelled)
	f.setState(t, 7, store.InReview)
	m := f.mapOf(t, "acme-1")
	// decided: chart and grilling Done; total: the four Wayfinder Tickets
	// not Cancelled; needs_you: research and the Prototype In Review.
	if *m != (core.MapView{Decided: 2, Total: 4, NeedsYou: 2, CanWriteSpec: false}) {
		t.Fatalf("map %+v", m)
	}
	for _, n := range []int{4, 5, 7, 8} {
		f.setState(t, n, store.Done)
	}
	if m := f.mapOf(t, "acme-1"); *m != (core.MapView{Decided: 4, Total: 4, NeedsYou: 0, CanWriteSpec: true}) {
		t.Fatalf("a Map with nothing unfinished can write its Spec: %+v", m)
	}
}

func TestEachTicketRowOnAMapCarriesItsNodeType(t *testing.T) {
	f := newFixture(t)
	f.mapWith(t,
		core.CreateTicketInput{Title: "Which store", Labels: []string{"wayfinder:grilling"}},    // acme-3
		core.CreateTicketInput{Title: "Sync libraries", Labels: []string{"wayfinder:research"}}, // acme-4
		core.CreateTicketInput{Title: "Ask legal", Labels: []string{"ui", "wayfinder:task"}},    // acme-5
		core.CreateTicketInput{Title: "Try a cache", Labels: []string{"prototype"}},             // acme-6
		core.CreateTicketInput{Title: "Unlabelled", Labels: []string{"ui"}},                     // acme-7
	)
	f.spec(t, "acme", "Login") // acme-8
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-8", Title: "Form"})
	sp, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-1"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, row := range sp.Tickets {
		got = append(got, row.NodeType)
	}
	want := []string{"chart", "grilling", "research", "task", "prototype", "ticket"}
	if !slices.Equal(got, want) {
		t.Fatalf("node types %q, want %q", got, want)
	}
	rows, err := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == "acme-9" && row.NodeType != "" {
			t.Fatalf("a Ticket on a plain Spec has no node type: %+v", row)
		}
		if row.ID == "acme-4" && row.NodeType != "research" {
			t.Fatalf("find_tickets rows carry the node type too: %+v", row)
		}
	}
}

// The frontier is the Map's Draft Tickets that aren't blocked: only a
// Done Blocker clears, so a Cancelled one keeps a Ticket off it.
func TestTheFrontierFollowsDoneAndCancelledBlockersThroughBlocked(t *testing.T) {
	f := newFixture(t)
	f.mapWith(t,
		core.CreateTicketInput{Title: "First", Labels: []string{"wayfinder:grilling"}},       // acme-3
		core.CreateTicketInput{Title: "After first", Labels: []string{"wayfinder:grilling"}}, // acme-4
		core.CreateTicketInput{Title: "Dropped", Labels: []string{"wayfinder:task"}},         // acme-5
		core.CreateTicketInput{Title: "After dropped", Labels: []string{"wayfinder:task"}},   // acme-6
	)
	f.addBlocker(t, "acme-4", "acme-3")
	f.addBlocker(t, "acme-6", "acme-5")
	frontier := func() []string {
		t.Helper()
		sp, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-1"})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, row := range sp.Tickets {
			if row.State == store.Draft && !row.Blocked {
				out = append(out, row.ID)
			}
		}
		return out
	}
	if got := frontier(); !slices.Equal(got, []string{"acme-3", "acme-5"}) {
		t.Fatalf("frontier %q", got)
	}
	f.setState(t, 3, store.Done)
	f.setState(t, 5, store.Cancelled)
	if got := frontier(); !slices.Equal(got, []string{"acme-4"}) {
		t.Fatalf("a Done Blocker clears and a Cancelled one doesn't: frontier %q", got)
	}
}
