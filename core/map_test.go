package core_test

import (
	"maps"
	"reflect"
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
	if m == nil || !reflect.DeepEqual(*m, core.MapView{Decided: 0, Total: 1, NeedsYou: 0, CanWriteSpec: false, Requirements: []string{}}) {
		t.Fatalf("a new Map: %+v", m)
	}
	rows, err := f.core.FindSpecs(ctx, "acme", core.FindSpecsInput{})
	if err != nil || len(rows) != 2 || rows[0].Map == nil || rows[1].Map != nil {
		t.Fatalf("find_specs rows %+v, %v", rows, err)
	}
	if !reflect.DeepEqual(rows[0].Map, m) {
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
	if !reflect.DeepEqual(*m, core.MapView{Decided: 2, Total: 4, NeedsYou: 2, CanWriteSpec: false, Requirements: []string{}}) {
		t.Fatalf("map %+v", m)
	}
	for _, n := range []int{4, 5, 7, 8} {
		f.setState(t, n, store.Done)
	}
	if m := f.mapOf(t, "acme-1"); !reflect.DeepEqual(*m, core.MapView{Decided: 4, Total: 4, NeedsYou: 0, CanWriteSpec: true, Requirements: []string{}}) {
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

// A Map's Ticket rows carry their Blockers, the edges of the Map view;
// rows off a Map leave them out.
func TestEachTicketRowOnAMapCarriesItsBlockers(t *testing.T) {
	f := newFixture(t)
	f.mapWith(t,
		core.CreateTicketInput{Title: "First", Labels: []string{"wayfinder:grilling"}},  // acme-3
		core.CreateTicketInput{Title: "Second", Labels: []string{"wayfinder:research"}}, // acme-4
		core.CreateTicketInput{Title: "Both", Labels: []string{"wayfinder:task"}},       // acme-5
	)
	f.addBlocker(t, "acme-4", "acme-3")
	f.addBlocker(t, "acme-5", "acme-3")
	f.addBlocker(t, "acme-5", "acme-4")
	f.spec(t, "acme", "Login")                                                                                    // acme-6
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-6", Title: "Form"})                                    // acme-7
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-6", Title: "Submit", BlockedBy: []core.Ref{"acme-7"}}) // acme-8
	sp, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-1"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for _, row := range sp.Tickets {
		got[row.ID] = row.BlockedBy
	}
	want := map[string][]string{"acme-2": nil, "acme-3": nil, "acme-4": {"acme-3"}, "acme-5": {"acme-3", "acme-4"}}
	for id, w := range want {
		if !slices.Equal(got[id], w) {
			t.Fatalf("%s blocked_by %q, want %q", id, got[id], w)
		}
	}
	rows, err := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == "acme-8" && row.BlockedBy != nil {
			t.Fatalf("a Ticket on a plain Spec leaves its Blockers out: %+v", row)
		}
	}
}

// Write the Spec: a finished Map becomes a new Requirement with no
// grilling Stage, whose spec Stage works from the Map.
func TestWritingTheSpecFromAMapCreatesARequirementWithOnlyTheSpecAndTicketsStages(t *testing.T) {
	f := newFixture(t)
	f.mapWith(t) // Charting is acme-2
	f.setState(t, 2, store.Done)
	res, err := f.core.SubmitRequirement(ctx, "acme", core.SubmitRequirementInput{Text: darkMode, Map: "acme-1"})
	if err != nil {
		t.Fatalf("submit_requirement with map: %v", err)
	}
	if res.Spec.ID != "acme-3" || len(res.Stages) != 2 || res.Stages[0].ID != "acme-4" || res.Stages[1].ID != "acme-5" {
		t.Fatalf("result %+v", res)
	}
	sp, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-3"})
	if err != nil {
		t.Fatal(err)
	}
	if sp.Title != "Add a dark mode" || *sp.Body != darkMode || !maps.Equal(sp.Tags, map[string]string{"map": "acme-1"}) || len(sp.Labels) != 0 {
		t.Fatalf("spec %+v", sp)
	}
	specStage, tickets := f.get(t, "acme-4"), f.get(t, "acme-5")
	if !slices.Equal(specStage.Labels, []string{"stage:spec"}) || len(specStage.BlockedBy) != 0 || !specStage.Claimable ||
		specStage.Title != "Spec: Add a dark mode" || specStage.Moves[0].Via != "submit_requirement" {
		t.Fatalf("the spec Stage is unblocked: %+v", specStage)
	}
	if !slices.Equal(tickets.Labels, []string{"stage:tickets"}) || !slices.Equal(tickets.BlockedBy, []string{"acme-4"}) {
		t.Fatalf("the tickets Stage follows it: %+v", tickets)
	}
	for _, want := range []string{"acme-3", "acme-1", "Decisions so far", "resolution"} {
		if !strings.Contains(specStage.Description, want) {
			t.Fatalf("the spec Stage's description should name %q: %q", want, specStage.Description)
		}
	}
	if sp.Requirement == nil || sp.Requirement.Map != "acme-1" || len(sp.Requirement.Stages) != 2 ||
		sp.Requirement.Stages[0].Name != "spec" || sp.Requirement.Stages[1].Name != "tickets" {
		t.Fatalf("requirement block %+v", sp.Requirement)
	}
	if m := f.mapOf(t, "acme-1"); !slices.Equal(m.Requirements, []string{"acme-3"}) {
		t.Fatalf("the Map lists the Requirement made from it: %+v", m)
	}
	rows, err := f.core.FindSpecs(ctx, "acme", core.FindSpecsInput{Finished: true})
	if err != nil || len(rows) != 2 || rows[1].Requirement.Map != "acme-1" || !slices.Equal(rows[0].Map.Requirements, []string{"acme-3"}) {
		t.Fatalf("find_specs rows %+v, %v", rows, err)
	}
}

func TestARequirementNotMadeFromAMapNamesNoMap(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	if _, err := f.core.SubmitRequirement(ctx, "acme", core.SubmitRequirementInput{Text: darkMode}); err != nil {
		t.Fatal(err)
	}
	sp, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-1"})
	if err != nil || sp.Requirement.Map != "" || len(sp.Tags) != 0 {
		t.Fatalf("spec %+v, %v", sp, err)
	}
	// A map Tag set by hand names no Map while the grilling Stage is there.
	if _, err := f.core.SetTags(ctx, "acme", core.SetTagsInput{Target: "acme-1", Tags: map[string]string{"map": "acme-9"}}); err != nil {
		t.Fatal(err)
	}
	if sp, _ := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-1"}); sp.Requirement.Map != "" {
		t.Fatalf("requirement %+v", sp.Requirement)
	}
}

func TestWritingTheSpecIsRefusedForASpecThatIsNotAFinishedMap(t *testing.T) {
	f := newFixture(t)
	f.mapWith(t, core.CreateTicketInput{Title: "Which store", Labels: []string{"wayfinder:grilling"}}) // acme-3
	f.setState(t, 2, store.Done)
	f.spec(t, "acme", "Login") // acme-4, a plain Spec

	_, err := f.core.SubmitRequirement(ctx, "acme", core.SubmitRequirementInput{Text: darkMode, Map: "acme-4"})
	wantCode(t, err, core.Invalid, "map")
	// The Map still has a Draft decision Ticket.
	_, err = f.core.SubmitRequirement(ctx, "acme", core.SubmitRequirementInput{Text: darkMode, Map: "acme-1"})
	wantCode(t, err, core.ActionNotAllowed, "")
	if err == nil || !strings.Contains(err.Error(), "acme-3 (Draft)") {
		t.Fatalf("the refusal names the unfinished Ticket: %v", err)
	}
	f.store.View(func(r store.Reader) error {
		if len(r.Specs("acme")) != 2 || len(r.Tickets("acme")) != 2 {
			t.Fatal("a refused Requirement wrote something")
		}
		return nil
	})
	// A Cancelled Ticket is finished too.
	f.setState(t, 3, store.Cancelled)
	if _, err := f.core.SubmitRequirement(ctx, "acme", core.SubmitRequirementInput{Text: darkMode, Map: "acme-1"}); err != nil {
		t.Fatalf("a finished Map writes its Spec: %v", err)
	}
}

// A Map is guided only when its guided Tag is on: a missing Tag or any
// other value means off.
func TestTheMapBlockIsGuidedOnlyWhenItsGuidedTagIsOn(t *testing.T) {
	f := newFixture(t)
	f.mapWith(t)
	if m := f.mapOf(t, "acme-1"); m.Guided {
		t.Fatalf("a Map with no guided Tag isn't guided: %+v", m)
	}
	for value, want := range map[string]bool{"on": true, "On": false, "true": false, "off": false} {
		if _, err := f.core.SetTags(ctx, "acme", core.SetTagsInput{Target: "acme-1", Tags: map[string]string{"guided": value}}); err != nil {
			t.Fatalf("set_tags: %v", err)
		}
		if m := f.mapOf(t, "acme-1"); m.Guided != want {
			t.Fatalf("guided: %s gives guided %v", value, m.Guided)
		}
	}
	if _, err := f.core.SetTags(ctx, "acme", core.SetTagsInput{Target: "acme-1", Tags: map[string]string{"guided": "on"}}); err != nil {
		t.Fatalf("set_tags: %v", err)
	}
	if _, err := f.core.RemoveTags(ctx, "acme", core.RemoveTagsInput{Target: "acme-1", Keys: []string{"guided"}}); err != nil {
		t.Fatalf("remove_tags: %v", err)
	}
	if m := f.mapOf(t, "acme-1"); m.Guided {
		t.Fatalf("removing the guided Tag turns it off: %+v", m)
	}
	if _, err := f.core.SetTags(ctx, "acme", core.SetTagsInput{Target: "acme-1", Tags: map[string]string{"guided": "on"}}); err != nil {
		t.Fatalf("set_tags: %v", err)
	}
	rows, err := f.core.FindSpecs(ctx, "acme", core.FindSpecsInput{})
	if err != nil || !rows[0].Map.Guided {
		t.Fatalf("find_specs rows carry guided too: %+v, %v", rows, err)
	}
}

func TestADraftMapsChargingTicketIsDraftAndItsSpecStillExists(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	res, err := f.core.SubmitMap(ctx, "acme", core.SubmitMapInput{Text: offlineMode, Draft: true})
	if err != nil {
		t.Fatal(err)
	}
	if tk := f.get(t, core.Ref(res.Charting.ID)); tk.State != store.Draft || tk.Claimable {
		t.Fatalf("charting %+v", tk)
	}
	if _, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: core.Ref(res.Spec.ID)}); err != nil {
		t.Fatal(err)
	}
}
