package core_test

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// scratch is a small .scratch/ folder mapped by an agent: two feature
// folders, with Blockers inside and across them.
func scratch() core.ImportPayload {
	return core.ImportPayload{
		Specs: []core.ImportSpec{
			{Source: ".scratch/login/spec.md", Title: "Login", Body: "# Login\n", Labels: []string{"auth"}, Tags: map[string]string{"type": "prd"}},
			{Source: ".scratch/billing/", Title: "billing"},
		},
		Tickets: []core.ImportTicket{
			{Source: ".scratch/login/issues/01-form.md", Spec: ".scratch/login/spec.md", Title: "Form", State: store.Done,
				Description: "Build the form.\n", Tags: map[string]string{"scratch.status": "resolved"}},
			{Source: ".scratch/login/issues/02-api.md", Spec: ".scratch/login/spec.md", Title: "API", State: store.Ready,
				Labels: []string{"backend"}, BlockedBy: []string{".scratch/login/issues/01-form.md"}},
			{Source: ".scratch/billing/issues/01-invoices.md", Spec: ".scratch/billing/", Title: "Invoices", State: store.Cancelled,
				Replacements: []string{".scratch/billing/issues/02-pdf.md"}, Tags: map[string]string{"scratch.status": "Superseded by 02"}},
			{Source: ".scratch/billing/issues/02-pdf.md", Spec: ".scratch/billing/", Title: "PDF invoices",
				BlockedBy: []string{".scratch/login/issues/02-api.md"}},
			{Source: ".scratch/billing/issues/03-tax.md", Spec: ".scratch/billing/", Title: "Tax", State: store.Cancelled,
				Comment: "wontfix: out of scope\n"},
		},
	}
}

func (f fixture) importFixture(t *testing.T) {
	t.Helper()
	f.project(t, "acme")
	f.spec(t, "acme", "Existing") // acme-1, so imports start at acme-2
}

func TestADryRunReturnsThePlanAndWritesNothing(t *testing.T) {
	f := newFixture(t)
	f.importFixture(t)
	plan, err := f.core.Import(ctx, "acme", core.ImportInput{Payload: scratch(), DryRun: true})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !plan.DryRun || len(plan.Specs) != 2 {
		t.Fatalf("plan: %+v", plan)
	}
	// IDs come from the counter in source order: each folder's Spec, then its Tickets.
	login, billing := plan.Specs[0], plan.Specs[1]
	if login.ID != "acme-2" || login.Source != ".scratch/login/spec.md" || login.Title != "Login" ||
		!slices.Equal(login.Labels, []string{"auth"}) || login.Tags["source"] != ".scratch/login/spec.md" || login.Tags["type"] != "prd" {
		t.Fatalf("login spec: %+v", login)
	}
	var ids []string
	for _, s := range plan.Specs {
		for _, tk := range s.Tickets {
			ids = append(ids, tk.ID)
		}
	}
	if billing.ID != "acme-5" || !slices.Equal(ids, []string{"acme-3", "acme-4", "acme-6", "acme-7", "acme-8"}) {
		t.Fatalf("billing %s, ticket IDs %v", billing.ID, ids)
	}
	api := login.Tickets[1]
	if api.Source != ".scratch/login/issues/02-api.md" || api.Title != "API" || api.State != store.Ready ||
		!slices.Equal(api.Labels, []string{"backend"}) || api.Tags["source"] != api.Source || !slices.Equal(api.BlockedBy, []string{"acme-3"}) {
		t.Fatalf("api ticket: %+v", api)
	}
	if inv := billing.Tickets[0]; !slices.Equal(inv.Replacements, []string{"acme-7"}) {
		t.Fatalf("invoices: %+v", inv)
	}
	if pdf := billing.Tickets[1]; pdf.State != store.Draft {
		t.Fatalf("a Ticket with no State is Draft: %+v", pdf)
	}
	want := map[store.State]int{store.Draft: 1, store.Ready: 1, store.Done: 1, store.Cancelled: 2}
	if !maps.Equal(plan.StateCounts, want) {
		t.Fatalf("state counts %v, want %v", plan.StateCounts, want)
	}
	if !strings.Contains(plan.NextStep, "go-ahead") {
		t.Fatalf("next step %q", plan.NextStep)
	}
	f.store.View(func(r store.Reader) error {
		if len(r.Tickets("acme")) != 0 || len(r.Specs("acme")) != 1 {
			t.Fatal("a dry run wrote something")
		}
		return nil
	})
}

func TestAnImportWritesEverySpecAndTicket(t *testing.T) {
	f := newFixture(t)
	f.importFixture(t)
	res, err := f.core.Import(core.WithActor(ctx, core.Human), "acme", core.ImportInput{Payload: scratch()})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.DryRun || len(res.Specs) != 2 || !strings.Contains(res.NextStep, "Delete `.scratch/`") || !strings.Contains(res.NextStep, "commit") {
		t.Fatalf("result: %+v", res)
	}
	sp, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-2"})
	if err != nil || *sp.Body != "# Login\n" || sp.Tags["source"] != ".scratch/login/spec.md" || len(sp.Tickets) != 2 {
		t.Fatalf("spec: %+v, %v", sp, err)
	}
	form, _ := f.core.GetTicket(ctx, "acme", whole("acme-3"))
	if form.State != store.Done || form.Description != "Build the form.\n" || form.Spec.ID != "acme-2" || len(form.Claims) != 0 {
		t.Fatalf("form: %+v", form)
	}
	// One move, with no from, written as agent whoever calls.
	if len(form.Moves) != 1 || form.Moves[0] != (core.MoveView{To: store.Done, Actor: "agent", Time: t0, Via: "import"}) {
		t.Fatalf("moves: %+v", form.Moves)
	}
	if api, _ := f.core.GetTicket(ctx, "acme", whole("acme-4")); api.Blocked || !api.Claimable {
		t.Fatalf("API is blocked only by Done acme-3: %+v", api)
	}
	pdf, _ := f.core.GetTicket(ctx, "acme", whole("acme-7"))
	if !pdf.Blocked || !slices.Equal(pdf.BlockedBy, []string{"acme-4"}) {
		t.Fatalf("pdf: %+v", pdf)
	}
	tax, _ := f.core.GetTicket(ctx, "acme", whole("acme-8"))
	if tax.State != store.Cancelled || len(tax.Comments) != 1 || tax.Comments[0].Body != "wontfix: out of scope\n" || tax.Comments[0].Actor != "agent" {
		t.Fatalf("tax: %+v", tax)
	}
	if spec := f.spec(t, "acme", "After"); spec.ID != "acme-9" {
		t.Fatalf("the counter after an import gave %s", spec.ID)
	}
}

func TestAnImportSendsOneProjectChangeNotice(t *testing.T) {
	f := newFixture(t)
	f.importFixture(t)
	notices, stop := f.core.Events().Subscribe()
	defer stop()
	if _, err := f.core.Import(ctx, "acme", core.ImportInput{Payload: scratch(), DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.Import(ctx, "acme", core.ImportInput{Payload: scratch()}); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-notices:
		if e != (events.Event{Type: events.ProjectChanged, Project: "acme"}) {
			t.Fatalf("notice %+v", e)
		}
	default:
		t.Fatal("no notice after the import")
	}
	noNotice(t, notices)
}

func TestAnImportIntoAnUnknownProjectIsRefused(t *testing.T) {
	f := newFixture(t)
	_, err := f.core.Import(ctx, "nope", core.ImportInput{Payload: scratch(), DryRun: true})
	wantCode(t, err, core.UnknownProject, "")
}

func TestAnImportCanRunOnlyOnce(t *testing.T) {
	f := newFixture(t)
	f.importFixture(t)
	if _, err := f.core.Import(ctx, "acme", core.ImportInput{Payload: scratch()}); err != nil {
		t.Fatal(err)
	}
	for _, dry := range []bool{true, false} {
		_, err := f.core.Import(ctx, "acme", core.ImportInput{Payload: scratch(), DryRun: dry})
		wantCode(t, err, core.Invalid, "specs[0].source")
		msg := err.(*core.Error).Message
		if !strings.Contains(msg, ".scratch/login/spec.md (acme-2)") || !strings.Contains(msg, ".scratch/billing/issues/03-tax.md (acme-8)") {
			t.Fatalf("message doesn't list the matches: %s", msg)
		}
	}
	// Removing one source Tag by hand lets that file be imported again.
	if _, err := f.core.RemoveTags(ctx, "acme", core.RemoveTagsInput{Target: "acme-8", Keys: []string{"source"}}); err != nil {
		t.Fatal(err)
	}
	p := scratch()
	p.Tickets = []core.ImportTicket{p.Tickets[4]}
	p.Tickets[0].Spec = ".scratch/tax/"
	p.Specs = []core.ImportSpec{{Source: ".scratch/tax/", Title: "tax"}}
	if _, err := f.core.Import(ctx, "acme", core.ImportInput{Payload: p}); err != nil {
		t.Fatalf("re-import after removing the source Tag: %v", err)
	}
}

func TestAnInvalidPayloadNamesTheFieldAndWritesNothing(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(p *core.ImportPayload)
		field string
	}{
		{"no specs", func(p *core.ImportPayload) { p.Specs, p.Tickets = nil, nil }, "specs"},
		{"spec without source", func(p *core.ImportPayload) { p.Specs[1].Source = "" }, "specs[1].source"},
		{"multi-line source", func(p *core.ImportPayload) { p.Specs[1].Source = "a\nb" }, "specs[1].source"},
		{"spec without title", func(p *core.ImportPayload) { p.Specs[0].Title = " " }, "specs[0].title"},
		{"empty label", func(p *core.ImportPayload) { p.Specs[0].Labels = []string{"ok", ""} }, "specs[0].labels[1]"},
		{"bad tag key", func(p *core.ImportPayload) { p.Specs[0].Tags = map[string]string{"Type": "prd"} }, "specs[0].tags.Type"},
		{"empty tag value", func(p *core.ImportPayload) { p.Tickets[2].Tags["scratch.status"] = "" }, "tickets[2].tags.scratch.status"},
		{"source tag given", func(p *core.ImportPayload) { p.Tickets[0].Tags["source"] = "x" }, "tickets[0].tags.source"},
		{"duplicate source", func(p *core.ImportPayload) { p.Tickets[3].Source = p.Tickets[1].Source }, "tickets[3].source"},
		{"ticket without title", func(p *core.ImportPayload) { p.Tickets[1].Title = "" }, "tickets[1].title"},
		{"unknown spec", func(p *core.ImportPayload) { p.Tickets[1].Spec = ".scratch/nope/" }, "tickets[1].spec"},
		{"in progress", func(p *core.ImportPayload) { p.Tickets[1].State = store.InProgress }, "tickets[1].state"},
		{"unknown state", func(p *core.ImportPayload) { p.Tickets[1].State = "Doing" }, "tickets[1].state"},
		{"unknown blocker", func(p *core.ImportPayload) { p.Tickets[3].BlockedBy = []string{"x", ".scratch/nope.md"} }, "tickets[3].blocked_by[0]"},
		{"spec as blocker", func(p *core.ImportPayload) { p.Tickets[3].BlockedBy = []string{".scratch/billing/"} }, "tickets[3].blocked_by[0]"},
		{"replacements when not cancelled", func(p *core.ImportPayload) { p.Tickets[1].Replacements = []string{p.Tickets[3].Source} }, "tickets[1].replacements"},
		{"unknown replacement", func(p *core.ImportPayload) { p.Tickets[2].Replacements = []string{p.Tickets[3].Source, "x"} }, "tickets[2].replacements[1]"},
		{"replaces itself", func(p *core.ImportPayload) { p.Tickets[2].Replacements = []string{p.Tickets[2].Source} }, "tickets[2].replacements[0]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			f.importFixture(t)
			p := scratch()
			c.edit(&p)
			_, err := f.core.Import(ctx, "acme", core.ImportInput{Payload: p})
			wantCode(t, err, core.Invalid, c.field)
			f.store.View(func(r store.Reader) error {
				if len(r.Tickets("acme")) != 0 || len(r.Specs("acme")) != 1 {
					t.Fatal("an invalid import wrote something")
				}
				return nil
			})
		})
	}
}

func TestACycleAmongImportedTicketsIsRefused(t *testing.T) {
	cases := []struct {
		name string
		edit func(p *core.ImportPayload)
	}{
		{"blocks itself", func(p *core.ImportPayload) { p.Tickets[1].BlockedBy = []string{p.Tickets[1].Source} }},
		{"two Tickets wait on each other", func(p *core.ImportPayload) { p.Tickets[1].BlockedBy = []string{p.Tickets[3].Source} }},
		// 02-api waits on Cancelled 01-invoices, so on its Replacement 02-pdf, which waits on 02-api.
		{"through a Replacement", func(p *core.ImportPayload) { p.Tickets[1].BlockedBy = []string{p.Tickets[2].Source} }},
		{"Replacements in a loop", func(p *core.ImportPayload) {
			p.Tickets[4].Replacements = []string{p.Tickets[2].Source}
			p.Tickets[2].Replacements = []string{p.Tickets[4].Source}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			f.importFixture(t)
			p := scratch()
			c.edit(&p)
			_, err := f.core.Import(ctx, "acme", core.ImportInput{Payload: p, DryRun: true})
			wantCode(t, err, core.Cycle, "")
		})
	}
}

func TestTheImportGuidePublishesTheMappingRules(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	g, err := f.core.GetImportGuide(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"spec.md", "map.md", "ready-for-agent", "wontfix", "Deferred", "Superseded",
		"scratch.status", "`source`", "Labels:", "dry_run", "doubts", "go-ahead"} {
		if !strings.Contains(g.Guide, want) {
			t.Errorf("the guide doesn't mention %q", want)
		}
	}
	if _, err := f.core.GetImportGuide(ctx, "nope"); err == nil {
		t.Fatal("an unknown Project got the guide")
	}
	if !strings.Contains(core.Instructions, "To import a `.scratch/` folder, call `get_import_guide`.") {
		t.Fatal("the MCP instructions don't point at get_import_guide")
	}
}
