package core_test

import (
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// requirement is a Project acme with a plain Spec acme-1 and the
// Requirement acme-2, whose Stages are acme-3 (grilling), acme-4 (spec)
// and acme-5 (tickets).
func (f fixture) requirement(t *testing.T) {
	t.Helper()
	f.project(t, "acme")
	f.spec(t, "acme", "Login")
	if _, err := f.core.SubmitRequirement(ctx, "acme", core.SubmitRequirementInput{Text: darkMode}); err != nil {
		t.Fatalf("submit_requirement: %v", err)
	}
	f.clock.Advance(time.Minute)
}

func (f fixture) requirementOf(t *testing.T, spec core.Ref) *core.RequirementView {
	t.Helper()
	sp, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: spec})
	if err != nil {
		t.Fatalf("get_spec %s: %v", spec, err)
	}
	return sp.Requirement
}

func stageLines(r *core.RequirementView) []string {
	var out []string
	for _, s := range r.Stages {
		out = append(out, s.Name+" "+s.Ticket+" "+string(s.State)+" "+string(s.NeedsMeReason))
	}
	return out
}

func TestASpecWithStageTicketsIsARequirement(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	if r := f.requirementOf(t, "acme-1"); r != nil {
		t.Fatalf("a Spec made with create_spec is not a Requirement: %+v", r)
	}
	r := f.requirementOf(t, "acme-2")
	if r == nil {
		t.Fatal("a submitted Requirement should carry a requirement block")
	}
	want := []string{"grilling acme-3 Ready ", "spec acme-4 Ready ", "tickets acme-5 Ready "}
	if got := stageLines(r); !slices.Equal(got, want) {
		t.Fatalf("stages %q, want %q", got, want)
	}
	if r.Build != (core.BuildProgress{Done: 0, Total: 0}) {
		t.Fatalf("build %+v, want 0/0", r.Build)
	}
	if !r.WaitingForFactory {
		t.Fatal("a new Requirement waits for a Factory to claim Grilling")
	}
}

func TestARequirementWhoseStagesAreAllCancelledIsStillARequirement(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	for _, n := range []int{3, 4, 5} {
		f.setState(t, n, store.Cancelled)
	}
	r := f.requirementOf(t, "acme-2")
	if r == nil {
		t.Fatal("a Requirement with only Cancelled Stages is still a Requirement")
	}
	if r.WaitingForFactory {
		t.Fatal("nothing can be claimed on a Requirement whose Stages are Cancelled")
	}
}

func TestARequirementWaitsForAFactoryWhileItsCurrentStageIsClaimable(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	res := f.claim(t, core.ClaimInput{Spec: "acme-2"})
	if res.Ticket.ID != "acme-3" {
		t.Fatalf("claimed %s, want the grilling Stage", res.Ticket.ID)
	}
	if f.requirementOf(t, "acme-2").WaitingForFactory {
		t.Fatal("a claimed Stage is not waiting for a Factory")
	}
	f.clock.Advance(core.SilentAfter)
	r := f.requirementOf(t, "acme-2")
	if r.WaitingForFactory || r.Stages[0].NeedsMeReason != core.ReasonSilentClaim {
		t.Fatalf("a Silent Claim needs the human, not a Factory: %+v", r)
	}
	f.setState(t, 3, store.InReview)
	r = f.requirementOf(t, "acme-2")
	if r.WaitingForFactory || r.Stages[0].NeedsMeReason != core.ReasonInReview {
		t.Fatalf("a Stage In Review waits on the human: %+v", r)
	}
	f.setState(t, 3, store.Done)
	r = f.requirementOf(t, "acme-2")
	if !r.WaitingForFactory {
		t.Fatalf("once Grilling is Done, the spec Stage waits for a Factory: %q", stageLines(r))
	}
}

func TestInBuildARequirementWaitsForAFactoryOnlyWhenNothingHoldsAClaim(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	for _, n := range []int{3, 4, 5} {
		f.setState(t, n, store.Done)
	}
	if f.requirementOf(t, "acme-2").WaitingForFactory {
		t.Fatal("all Stages Done with no implementation Tickets: nothing to claim")
	}
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "2", Title: "Toggle", State: store.Ready})   // acme-6
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "2", Title: "Remember", State: store.Ready}) // acme-7
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "2", Title: "Docs", State: store.Ready})     // acme-8
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "2", Title: "Dropped", State: store.Ready})  // acme-9
	f.setState(t, 8, store.Done)
	f.setState(t, 9, store.Cancelled)
	r := f.requirementOf(t, "acme-2")
	if !r.WaitingForFactory {
		t.Fatal("claimable implementation Tickets and no Claim: waiting for a Factory")
	}
	if r.Build != (core.BuildProgress{Done: 1, Total: 3}) {
		t.Fatalf("build %+v, want 1/3 leaving out the Cancelled Ticket", r.Build)
	}
	f.claim(t, core.ClaimInput{Ticket: "acme-6"})
	if f.requirementOf(t, "acme-2").WaitingForFactory {
		t.Fatal("a held Claim means a Factory is at work, though acme-7 is still claimable")
	}
}

func TestNeedsMeRowsOnARequirementNameItAndTheirStage(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "2", Title: "Toggle", State: store.Ready}) // acme-6
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Form", State: store.Ready})   // acme-7
	for _, n := range []int{3, 6, 7} {
		f.setState(t, n, store.InReview)
	}
	v, err := f.core.NeedsMe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*core.NeedsMeRequirement{}
	for _, g := range v.Groups {
		for _, row := range g.Tickets {
			got[row.ID] = row.Requirement
		}
	}
	if r := got["acme-3"]; r == nil || *r != (core.NeedsMeRequirement{Spec: "acme-2", Title: "Add a dark mode", Stage: "grilling"}) {
		t.Fatalf("Stage row requirement %+v", r)
	}
	if r := got["acme-6"]; r == nil || *r != (core.NeedsMeRequirement{Spec: "acme-2", Title: "Add a dark mode"}) {
		t.Fatalf("implementation row requirement %+v", r)
	}
	if r, ok := got["acme-7"]; !ok || r != nil {
		t.Fatalf("a Ticket on a plain Spec has no requirement: %+v (present %v)", r, ok)
	}
}

func TestFindSpecsCanKeepOnlyRequirements(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	all, err := f.core.FindSpecs(ctx, "acme", core.FindSpecsInput{})
	if err != nil || len(all) != 2 || all[0].Requirement != nil || all[1].Requirement == nil {
		t.Fatalf("find_specs rows %+v, %v", all, err)
	}
	if !all[1].Requirement.WaitingForFactory || len(all[1].Requirement.Stages) != 3 {
		t.Fatalf("row requirement %+v", all[1].Requirement)
	}
	only, err := f.core.FindSpecs(ctx, "acme", core.FindSpecsInput{Requirements: true})
	if err != nil || len(only) != 1 || only[0].ID != "acme-2" {
		t.Fatalf("find_specs requirements: %+v, %v", only, err)
	}
}

func TestNeedsMeListsMapTicketsAndPrototypesOneByOneWithTheirSpec(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	f.spec(t, "acme", "Find the way") // acme-6
	if _, err := f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: "acme-6", Labels: []string{core.MapLabel}}); err != nil {
		t.Fatal(err)
	}
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "6", Title: "Charting", Labels: []string{"wayfinder:chart"}, State: store.Ready}) // acme-7
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "6", Title: "Which store", Labels: []string{"wayfinder:grilling"}})               // acme-8
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "6", Title: "Try the graph", Labels: []string{"prototype"}, State: store.Ready})  // acme-9
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Try the form", Labels: []string{"prototype"}, State: store.Ready})   // acme-10
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "2", Title: "Try the toggle", Labels: []string{"prototype"}, State: store.Ready}) // acme-11
	for _, n := range []int{7, 8, 9, 10, 11} {
		f.setState(t, n, store.InReview)
	}
	v, err := f.core.NeedsMe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]core.NeedsMeRow{}
	for _, g := range v.Groups {
		for _, row := range g.Tickets {
			got[row.ID] = row
		}
	}
	mapSpec := core.NeedsMeSpec{Spec: "acme-6", Title: "Find the way", Map: true}
	for id, want := range map[string]struct {
		spec      *core.NeedsMeSpec
		prototype bool
	}{
		"acme-7":  {&mapSpec, false},
		"acme-8":  {&mapSpec, false},
		"acme-9":  {&mapSpec, true},
		"acme-10": {&core.NeedsMeSpec{Spec: "acme-1", Title: "Login"}, true},
		// On a Requirement, the Requirement is its context.
		"acme-11": {nil, true},
	} {
		row, ok := got[id]
		if !ok {
			t.Fatalf("%s isn't its own Needs Me row", id)
		}
		if row.Prototype != want.prototype || (row.Spec == nil) != (want.spec == nil) || (row.Spec != nil && *row.Spec != *want.spec) {
			t.Errorf("%s spec %+v prototype %v, want %+v %v", id, row.Spec, row.Prototype, want.spec, want.prototype)
		}
		if row.Requirement != nil && (id != "acme-11" || row.Requirement.Stage != "") {
			t.Errorf("%s requirement %+v", id, row.Requirement)
		}
	}
	if got["acme-11"].Requirement == nil {
		t.Error("a Prototype on a Requirement names it")
	}
}

func TestACancelledTicketsStageCountsAsPassedSoBuildIsCurrent(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	f.setState(t, 3, store.Done)
	f.setState(t, 4, store.Done)
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "2", Title: "Everything", State: store.Ready}) // acme-6
	f.cancel(t, core.CancelInput{Ticket: "acme-5", Replacements: []core.Ref{"acme-6"}}, core.Human)
	r := f.requirementOf(t, "acme-2")
	want := []string{"grilling acme-3 Done ", "spec acme-4 Done ", "tickets acme-5 Cancelled "}
	if got := stageLines(r); !slices.Equal(got, want) {
		t.Fatalf("stages %q, want %q", got, want)
	}
	if r.Build != (core.BuildProgress{Done: 0, Total: 1}) || !r.WaitingForFactory {
		t.Fatalf("build %+v waiting %v, want 0/1 waiting for the implementation Ticket", r.Build, r.WaitingForFactory)
	}
	f.claim(t, core.ClaimInput{Ticket: "acme-6"})
	if f.requirementOf(t, "acme-2").WaitingForFactory {
		t.Fatal("a held Claim on the implementation Ticket means a Factory is at work")
	}
	f.setState(t, 6, store.Done)
	if r := f.requirementOf(t, "acme-2"); r.Build != (core.BuildProgress{Done: 1, Total: 1}) {
		t.Fatalf("build %+v, want 1/1", r.Build)
	}
}
