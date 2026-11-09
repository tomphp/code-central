package core_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Reserved Wayfinder Labels and their Guides (Spec 213, ticket 217).

// ticketTypeLabels are the reserved Labels that give a Ticket its type,
// with the Guide each one gets.
var ticketTypeLabels = map[string]string{
	"wayfinder:chart":    "map-chart",
	"wayfinder:grilling": "map-grilling",
	"wayfinder:research": "map-research",
	"wayfinder:task":     "map-task",
	"prototype":          "prototype",
}

func TestTheWayfinderLabelsAndPrototypeAreAcceptedWhereTheyBelong(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Map") // acme-1
	for l := range ticketTypeLabels {
		f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: l, Labels: []string{"ui", l}})
		tk := f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: l + " later"})
		if _, err := f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: core.Ref(tk.ID), Labels: []string{l}}); err != nil {
			t.Errorf("add_labels %s on a Ticket: %v", l, err)
		}
	}
	if _, err := f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: "acme-1", Labels: []string{"wayfinder:map"}}); err != nil {
		t.Errorf("add_labels wayfinder:map on a Spec: %v", err)
	}

	p := scratch()
	p.Specs[0].Labels = []string{"wayfinder:map"}
	p.Tickets[0].Labels = []string{"wayfinder:chart"}
	p.Tickets[1].Labels = []string{"backend", "prototype"}
	if _, err := f.core.Import(ctx, "acme", core.ImportInput{Payload: p, DryRun: true}); err != nil {
		t.Errorf("import with Wayfinder Labels where they belong: %v", err)
	}
}

func TestAnUnknownWayfinderNameIsRejectedListingTheValidOnes(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Map") // acme-1
	tk := f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Form"})

	wantList := func(err error) {
		t.Helper()
		var e *core.Error
		if !errors.As(err, &e) {
			t.Fatalf("err = %v, want a core.Error", err)
		}
		for _, name := range []string{"wayfinder:map", "wayfinder:chart", "wayfinder:grilling", "wayfinder:research", "wayfinder:task"} {
			if !strings.Contains(e.Message, name) {
				t.Errorf("message %q doesn't list %s", e.Message, name)
			}
		}
	}
	for _, l := range []string{"wayfinder:reseach", "wayfinder:", "wayfinder:Map", "wayfinder:prototype"} {
		_, err := f.core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Form", Labels: []string{"ui", l}})
		wantCode(t, err, core.Invalid, "labels")
		wantList(err)
		_, err = f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: core.Ref(tk.ID), Labels: []string{l}})
		wantCode(t, err, core.Invalid, "labels")
		_, err = f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: "acme-1", Labels: []string{l}})
		wantCode(t, err, core.Invalid, "labels")
	}

	p := scratch()
	p.Tickets[1].Labels = []string{"backend", "wayfinder:reseach"}
	_, err := f.core.Import(ctx, "acme", core.ImportInput{Payload: p, DryRun: true})
	wantCode(t, err, core.Invalid, "tickets[1].labels[1]")
	wantList(err)
	p = scratch()
	p.Specs[0].Labels = []string{"wayfinder:mpa"}
	_, err = f.core.Import(ctx, "acme", core.ImportInput{Payload: p, DryRun: true})
	wantCode(t, err, core.Invalid, "specs[0].labels[0]")

	// Labels that only look like the prefix aren't reserved.
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Free", Labels: []string{"wayfinder", "my-wayfinder:x", "prototypes", "Prototype"}})
}

func TestTheMapLabelIsOnlyForSpecsAndTheTicketTypesOnlyForTickets(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Map") // acme-1
	tk := f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Form"})

	_, err := f.core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Form", Labels: []string{"wayfinder:map"}})
	wantCode(t, err, core.Invalid, "labels")
	_, err = f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: core.Ref(tk.ID), Labels: []string{"wayfinder:map"}})
	wantCode(t, err, core.Invalid, "labels")
	for l := range ticketTypeLabels {
		_, err = f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: "acme-1", Labels: []string{l}})
		wantCode(t, err, core.Invalid, "labels")
	}

	p := scratch()
	p.Tickets[1].Labels = []string{"backend", "wayfinder:map"}
	_, err = f.core.Import(ctx, "acme", core.ImportInput{Payload: p, DryRun: true})
	wantCode(t, err, core.Invalid, "tickets[1].labels[1]")
	for l := range ticketTypeLabels {
		p = scratch()
		p.Specs[0].Labels = []string{"auth", l}
		_, err = f.core.Import(ctx, "acme", core.ImportInput{Payload: p, DryRun: true})
		wantCode(t, err, core.Invalid, "specs[0].labels[1]")
	}
}

func TestATicketCarriesAtMostOneReservedTypeLabel(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Map") // acme-1

	for _, pair := range [][]string{
		{"stage:spec", "wayfinder:task"},
		{"stage:grilling", "stage:tickets"},
		{"wayfinder:chart", "wayfinder:grilling"},
		{"wayfinder:research", "prototype"},
		{"stage:tickets", "prototype"},
	} {
		_, err := f.core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Two", Labels: append([]string{"ui"}, pair...)})
		wantCode(t, err, core.Invalid, "labels")

		tk := f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "One", Labels: pair[:1]})
		_, err = f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: core.Ref(tk.ID), Labels: pair[1:]})
		wantCode(t, err, core.Invalid, "labels")
		_, err = f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: core.Ref(tk.ID), Labels: []string{"ui", pair[0]}})
		if err != nil {
			t.Errorf("add_labels of the Label it already has, %s: %v", pair[0], err)
		}

		p := scratch()
		p.Tickets[0].Labels = pair
		_, err = f.core.Import(ctx, "acme", core.ImportInput{Payload: p, DryRun: true})
		wantCode(t, err, core.Invalid, "tickets[0].labels")
	}
	// The same Label twice is still one.
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Twice", Labels: []string{"prototype", "prototype"}})
	// A Spec isn't limited: the rule is per Ticket.
	if _, err := f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: "acme-1", Labels: []string{"wayfinder:map", "stage:spec"}}); err != nil {
		t.Errorf("add_labels on a Spec: %v", err)
	}
}

func TestTheGuideNameComesFromTheTicketsReservedTypeLabel(t *testing.T) {
	for _, c := range []struct {
		labels []string
		want   string
	}{
		{nil, "implementation"},
		{[]string{"ui", "backend"}, "implementation"},
		{[]string{"stage:grilling"}, "grilling"},
		{[]string{"ui", "stage:spec"}, "spec"},
		{[]string{"stage:tickets"}, "tickets"},
		{[]string{"ui", "wayfinder:chart"}, "map-chart"},
		{[]string{"wayfinder:grilling"}, "map-grilling"},
		{[]string{"wayfinder:research"}, "map-research"},
		{[]string{"wayfinder:task", "ui"}, "map-task"},
		{[]string{"prototype"}, "prototype"},
	} {
		if got := core.GuideName(c.labels); got != c.want {
			t.Errorf("GuideName(%q) = %q, want %q", c.labels, got, c.want)
		}
	}
}

func TestClaimCarriesTheGuideForEachWayfinderTypeAndAPrototype(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Map")
	for l, name := range ticketTypeLabels {
		tk := f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: l, State: store.Ready, Labels: []string{l}})
		res, err := f.core.Claim(ctx, "acme", core.ClaimInput{Ticket: core.Ref(tk.ID)})
		if err != nil {
			t.Fatal(err)
		}
		if res.StageGuide == nil || res.StageGuide.Stage != name || res.StageGuide.Guide != guide(t, f, name) {
			t.Errorf("claim of a %s Ticket carried %+v, want the %s guide", l, res.StageGuide, name)
		}
	}
}

func TestTheChartingGuideWritesTheMapsSectionsAndItsFirstTickets(t *testing.T) {
	f := newFixture(t)
	wantAll(t, "map-chart", guide(t, f, "map-chart"),
		"Rounds", "destination",
		"`edit_spec`", "`## Destination`", "`## Notes`", "`## Decisions so far`", "`## Not yet specified`", "`## Out of scope`",
		"`create_ticket`", "`wayfinder:grilling`", "`wayfinder:research`", "`wayfinder:task`",
		"grilling and task Tickets in `Draft`", "research Tickets `state` `Ready`", "`add_blocker`",
		"resolution comment", "`comment`", "Give no `branch` or `commit`",
	)
}

func TestTheMapGrillingGuideRecordsItsDecisionInTheMap(t *testing.T) {
	f := newFixture(t)
	wantAll(t, "map-grilling", guide(t, f, "map-grilling"),
		"one decision", "`edit_spec`",
		"`## Decisions so far`", "a link to this Ticket", "`## Not yet specified`", "`## Out of scope`",
		"`create_ticket`", "`add_blocker`",
		"resolution comment", "`comment`", "Give no `branch` or `commit`",
	)
}

func TestTheMapResearchGuideCommitsFindingsAndFinishesWithNoBranch(t *testing.T) {
	f := newFixture(t)
	wantAll(t, "map-research", guide(t, f, "map-research"),
		"Commit the findings", "research branch", "Don't push",
		"`set_tags`", "`research`", "`<branch>@<commit>`",
		"resolution comment", "summary of the findings", "the branch that holds them",
		"`## Decisions so far`", "Give no `branch`",
	)
}

func TestTheMapTaskGuideWorksThroughOperationsOrAsksForAChecklist(t *testing.T) {
	f := newFixture(t)
	wantAll(t, "map-task", guide(t, f, "map-task"),
		"through Code Central operations",
		"only the human can do it", "`post_round`", "checklist",
		"resolution comment", "`comment`", "Give no `branch` or `commit`",
	)
}

func TestThePrototypeGuideBuildsTriesAndFinishesWithNoBranch(t *testing.T) {
	f := newFixture(t)
	wantAll(t, "prototype", guide(t, f, "prototype"),
		"commit it on the prototype branch", "Don't push",
		"`set_tags`", "`prototype`", "`<branch>@<commit> <worktree path>`",
		"\"Try it\" as the first Question", "recommended answer", "iterate",
		"resolution comment", "what was learned", "which decision it informs",
		"Give no `branch`",
	)
}
