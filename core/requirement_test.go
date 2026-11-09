package core_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

const darkMode = "Add a dark mode\n\nThe settings page gets a toggle, and the choice is remembered.\n"

func TestSubmittingARequirementCreatesItsSpecAndThreeChainedStages(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Existing") // acme-1, so the Requirement starts at acme-2
	res, err := f.core.SubmitRequirement(ctx, "acme", core.SubmitRequirementInput{Text: darkMode})
	if err != nil {
		t.Fatalf("submit_requirement: %v", err)
	}
	sp, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-2"})
	if err != nil {
		t.Fatalf("get_spec: %v", err)
	}
	if res.Spec.ID != "acme-2" || sp.Title != "Add a dark mode" || *sp.Body != darkMode {
		t.Fatalf("spec: %+v (result %+v)", sp, res.Spec)
	}
	want := []struct {
		id, label string
		blockedBy []string
	}{
		{"acme-3", "stage:grilling", []string{}},
		{"acme-4", "stage:spec", []string{"acme-3"}},
		{"acme-5", "stage:tickets", []string{"acme-4"}},
	}
	if len(res.Stages) != len(want) {
		t.Fatalf("stages %+v", res.Stages)
	}
	for i, w := range want {
		tk, err := f.core.GetTicket(ctx, "acme", whole(core.Ref(w.id)))
		if err != nil {
			t.Fatalf("get_ticket %s: %v", w.id, err)
		}
		if res.Stages[i].ID != w.id || tk.State != store.Ready || tk.Spec.ID != "acme-2" ||
			!slices.Equal(tk.Labels, []string{w.label}) || !slices.Equal(tk.BlockedBy, w.blockedBy) {
			t.Fatalf("stage %d: %+v", i, tk)
		}
	}
	grilling, _ := f.core.GetTicket(ctx, "acme", whole("acme-3"))
	if !strings.Contains(grilling.Description, "acme-2") {
		t.Fatalf("the grilling Ticket's description should point at the Spec: %q", grilling.Description)
	}
	if !grilling.Claimable {
		t.Fatal("the grilling Ticket should be claimable straight away")
	}
}

func TestARequirementIsWrittenAsTheHumanWhoeverSubmitsIt(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	// No Actor in the context is the agent, as over MCP and the CLI.
	if _, err := f.core.SubmitRequirement(ctx, "acme", core.SubmitRequirementInput{Text: darkMode}); err != nil {
		t.Fatalf("submit_requirement: %v", err)
	}
	for _, id := range []string{"acme-2", "acme-3", "acme-4"} {
		tk, _ := f.core.GetTicket(ctx, "acme", whole(core.Ref(id)))
		if len(tk.Moves) != 1 || tk.Moves[0].Actor != "human" || tk.Moves[0].To != store.Ready || tk.Moves[0].Via != "submit_requirement" {
			t.Fatalf("%s moves %+v", id, tk.Moves)
		}
	}
}

func TestARequirementNeedsText(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	for _, text := range []string{"", "  \n\t\n"} {
		_, err := f.core.SubmitRequirement(ctx, "acme", core.SubmitRequirementInput{Text: text})
		wantCode(t, err, core.Invalid, "text")
	}
	f.store.View(func(r store.Reader) error {
		if len(r.Specs("acme")) != 0 || len(r.Tickets("acme")) != 0 {
			t.Fatal("a rejected Requirement wrote something")
		}
		return nil
	})
}

func TestOnlyTheThreeStageNamesMayUseTheReservedStagePrefix(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Login") // acme-1

	_, err := f.core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Form", Labels: []string{"ui", "stage:build"}})
	wantCode(t, err, core.Invalid, "labels")
	tk := f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Form", Labels: []string{"stage:spec", "stages", "backstage:x"}})

	_, err = f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: core.Ref(tk.ID), Labels: []string{"stage:"}})
	wantCode(t, err, core.Invalid, "labels")
	_, err = f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: "acme-1", Labels: []string{"stage:implementation"}})
	wantCode(t, err, core.Invalid, "labels")
	if _, err := f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: core.Ref(tk.ID), Labels: []string{"stage:grilling", "stage:tickets"}}); err != nil {
		t.Fatalf("add_labels with known Stage names: %v", err)
	}

	p := scratch()
	p.Tickets[1].Labels = []string{"backend", "stage:review"}
	_, err = f.core.Import(ctx, "acme", core.ImportInput{Payload: p, DryRun: true})
	wantCode(t, err, core.Invalid, "tickets[1].labels[1]")
	p = scratch()
	p.Specs[0].Labels = []string{"stage:x"}
	_, err = f.core.Import(ctx, "acme", core.ImportInput{Payload: p, DryRun: true})
	wantCode(t, err, core.Invalid, "specs[0].labels[0]")
	p = scratch()
	p.Tickets[0].Labels = []string{"stage:grilling"}
	if _, err := f.core.Import(ctx, "acme", core.ImportInput{Payload: p, DryRun: true}); err != nil {
		t.Fatalf("import with a known Stage Label: %v", err)
	}
}

func TestARequirementsTitleIsItsFirstLine(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	text := "\n# Dark mode  \nMore.\n"
	res, err := f.core.SubmitRequirement(ctx, "acme", core.SubmitRequirementInput{Text: text, Full: true})
	if err != nil || res.Spec.Record.Title != "Dark mode" || res.Spec.Record.Body != text {
		t.Fatalf("spec %+v, %v", res.Spec, err)
	}
}

func TestSubmittingARequirementAcknowledgesItsSpecAndStages(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	res, err := f.core.SubmitRequirement(ctx, "acme", core.SubmitRequirementInput{Text: darkMode})
	if err != nil {
		t.Fatalf("submit_requirement: %v", err)
	}
	wantAcknowledgement(t, res.Spec, "acme-1", t0, []string{"title", "body"}, 0)
	if len(res.Stages) != 3 {
		t.Fatalf("stages %+v", res.Stages)
	}
	wantTicketAcknowledgement(t, res.Stages[0], "acme-2", t0, store.Ready,
		[]string{"title", "state", "spec", "labels", "description", "moves", "claimable"}, 0)
	for i, id := range []string{"acme-3", "acme-4"} {
		wantTicketAcknowledgement(t, res.Stages[i+1], id, t0, store.Ready,
			[]string{"title", "state", "spec", "labels", "blocked_by", "blockers", "description", "moves", "blocked"}, 0)
	}
}

func TestSubmittingARequirementWithFullReturnsTheSpecAndStages(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	res, err := f.core.SubmitRequirement(ctx, "acme", core.SubmitRequirementInput{Text: darkMode, Full: true})
	if err != nil {
		t.Fatalf("submit_requirement: %v", err)
	}
	out := resultJSON(t, res)
	sp, _ := out["spec"].(map[string]any)
	if sp["id"] != "acme-1" || sp["body"] != darkMode {
		t.Fatalf("spec %v", out["spec"])
	}
	stages, _ := out["stages"].([]any)
	if len(stages) != 3 {
		t.Fatalf("stages %v", out["stages"])
	}
	for i, label := range []string{"stage:grilling", "stage:spec", "stage:tickets"} {
		st := stages[i].(map[string]any)
		if st["state"] != "Ready" || !reflect.DeepEqual(st["labels"], []any{label}) || st["moves"] == nil {
			t.Fatalf("stage %d: %v", i, st)
		}
	}
}
