package core_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

func (f fixture) setModels(t *testing.T, in api.SetModelsInput) api.ProjectView {
	t.Helper()
	in.Full = true
	w, err := f.core.SetModels(human, "acme", in)
	if err != nil {
		t.Fatalf("set_models: %v", err)
	}
	return *w.Record
}

// row is a Models row setting model and effort.
func row(model, effort string) api.ModelRowView {
	return api.ModelRowView{Model: model, Effort: effort}
}

func kind(name, model, effort string) api.KindModelsView {
	return api.KindModelsView{Kind: name, ModelRowView: row(model, effort)}
}

func choice(model api.ChoiceValue, effort api.ChoiceValue) api.ModelChoice {
	return api.ModelChoice{Model: model, Effort: effort}
}

func set(v string, s api.ChoiceSource) api.ChoiceValue {
	return api.ChoiceValue{Value: v, Source: s}
}

// fromKind is a value the Models row for the kind of work named set.
func fromKind(v, name string) api.ChoiceValue {
	return api.ChoiceValue{Value: v, Source: api.SourceProjectKind, Kind: name}
}

var unset = api.ChoiceValue{Source: api.SourceNone}

// wantModels checks a Project's Models lists the default row, then every
// kind of work in order, each row as want sets it.
func wantModels(t *testing.T, got api.ModelsView, def api.ModelRowView, want ...api.KindModelsView) {
	t.Helper()
	if got.Default != def {
		t.Errorf("default row = %+v, want %+v", got.Default, def)
	}
	var kinds []string
	for _, k := range got.Kinds {
		kinds = append(kinds, k.Kind)
		w := api.KindModelsView{Kind: k.Kind}
		for _, x := range want {
			if x.Kind == k.Kind {
				w = x
			}
		}
		if k != w {
			t.Errorf("%s row = %+v, want %+v", k.Kind, k, w)
		}
	}
	if !slices.Equal(kinds, core.GuideNames) {
		t.Errorf("rows = %v, want one per kind of work: %v", kinds, core.GuideNames)
	}
}

func TestEveryProjectReadCarriesAModelsRowForEachKindOfWorkSettingNothing(t *testing.T) {
	f := newFixture(t)
	wantModels(t, f.project(t, "acme").Models, row("", ""))
}

func TestTheStoresKindsOfWorkAreTheGuideNames(t *testing.T) {
	if !slices.Equal(domain.ModelKinds, core.GuideNames) {
		t.Fatalf("store.ModelKinds = %v, want core.GuideNames %v", domain.ModelKinds, core.GuideNames)
	}
}

func TestSetModelsReplacesTheRowsWhole(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)
	p := f.setModels(t, api.SetModelsInput{Default: row(" opus ", "high"), Kinds: []api.KindModelsView{kind("implementation", "", "xhigh"), kind("grilling", "fable", "")}})
	wantModels(t, p.Models, row("opus", "high"), kind("grilling", "fable", ""), kind("implementation", "", "xhigh"))

	p = f.setModels(t, api.SetModelsInput{Kinds: []api.KindModelsView{kind("spec", "sonnet", ""), kind("prototype", "", "")}})
	wantModels(t, p.Models, row("", ""), kind("spec", "sonnet", ""))
}

func TestTheChatRowIsSeparateFromTheKindsOfWorkAndTheDefault(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)
	if p, _ := f.core.GetProject(ctx, "acme"); p.Models.Chat != row("", "") {
		t.Fatalf("chat row = %+v, want it empty before it is set", p.Models.Chat)
	}

	p := f.setModels(t, api.SetModelsInput{Default: row("opus", "high"), Chat: row(" sonnet ", "low")})
	if p.Models.Chat != row("sonnet", "low") {
		t.Errorf("chat row = %+v, want sonnet at low", p.Models.Chat)
	}
	wantModels(t, p.Models, row("opus", "high"))

	// The Chat row doesn't reach a Ticket's Model Choice, and the Project
	// default doesn't reach Chat's row.
	mc := f.modelChoice(t, "acme-2")
	if mc.Model.Value != "opus" || mc.Effort.Value != "high" {
		t.Errorf("model choice = %+v, want the default row's opus at high", mc)
	}

	// A Chat row alone survives, as the rows are replaced whole.
	p = f.setModels(t, api.SetModelsInput{Chat: row("", "max")})
	if p.Models.Chat != row("", "max") || p.Models.Default != row("", "") {
		t.Errorf("models = %+v, want only a Chat effort", p.Models)
	}
	p = f.setModels(t, api.SetModelsInput{})
	if p.Models.Chat != row("", "") {
		t.Errorf("chat row = %+v, want it cleared by rows that leave it out", p.Models.Chat)
	}
}

func TestSetModelsChecksTheChatRow(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	for _, c := range []struct {
		name  string
		in    api.SetModelsInput
		field string
	}{
		{"a blank model", api.SetModelsInput{Chat: row("  ", "")}, "chat.model"},
		{"a multi-line model", api.SetModelsInput{Chat: row("a\nb", "")}, "chat.model"},
		{"an effort outside the levels", api.SetModelsInput{Chat: row("", "extreme")}, "chat.effort"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := f.core.SetModels(human, "acme", c.in)
			wantCode(t, err, api.Invalid, c.field)
		})
	}
}

func TestSetModelsIsTheHumans(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	for _, a := range []api.Actor{api.Agent, api.Integration} {
		_, err := f.core.SetModels(api.WithActor(ctx, a), "acme", api.SetModelsInput{Default: row("opus", "")})
		wantCode(t, err, api.ActionNotAllowed, "")
	}
	if p, _ := f.core.GetProject(ctx, "acme"); p.Models.Default != row("", "") {
		t.Fatalf("a refused set_models changed the Models: %+v", p.Models)
	}
}

func TestSetModelsValidatesBeforeWriting(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.setModels(t, api.SetModelsInput{Default: row("opus", "")})
	for _, c := range []struct {
		name  string
		in    api.SetModelsInput
		field string
		says  []string
	}{
		{"a row for something else", api.SetModelsInput{Kinds: []api.KindModelsView{kind("review", "opus", "")}}, "kinds", []string{`"review"`, strings.Join(core.GuideNames, ", ")}},
		{"a kind twice", api.SetModelsInput{Kinds: []api.KindModelsView{kind("spec", "opus", ""), kind("spec", "", "low")}}, "kinds", []string{`"spec"`}},
		{"a blank model", api.SetModelsInput{Default: row("  ", "")}, "default.model", []string{"blank"}},
		{"a multi-line model", api.SetModelsInput{Kinds: []api.KindModelsView{kind("tickets", "opus\nsonnet", "")}}, "kinds.tickets.model", []string{"one line"}},
		{"an effort outside the levels", api.SetModelsInput{Kinds: []api.KindModelsView{kind("implementation", "", "extreme")}}, "kinds.implementation.effort", []string{`"extreme"`, "low, medium, high, xhigh, max"}},
		{"a default effort outside the levels", api.SetModelsInput{Default: row("", "High")}, "default.effort", []string{`"High"`}},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := f.core.SetModels(human, "acme", c.in)
			wantCode(t, err, api.Invalid, c.field)
			for _, s := range c.says {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("the refusal should say %q: %v", s, err)
				}
			}
			if p, _ := f.core.GetProject(ctx, "acme"); p.Models.Default != row("opus", "") {
				t.Errorf("a refused set_models changed the Models: %+v", p.Models)
			}
		})
	}
}

func TestSetModelsAddsNoMoveEditOrCommentAndSendsTheProjectsNotice(t *testing.T) {
	f := newFixture(t)
	before := f.readyTicket(t)
	notices, stop := f.core.Events().Subscribe()
	defer stop()

	w, err := f.core.SetModels(human, "acme", api.SetModelsInput{Default: row("opus", "")})
	if err != nil {
		t.Fatal(err)
	}
	if w.Record != nil || w.ID != "acme" || !slices.Equal(w.Changed, []string{"models"}) {
		t.Errorf("acknowledgement = %+v, want models changed on acme", w)
	}
	if got := <-notices; got != (events.Event{Type: events.ProjectChanged, Project: "acme"}) {
		t.Fatalf("got %+v, want the Project's notice", got)
	}
	noNotice(t, notices)

	// The same rows again write nothing.
	if _, err := f.core.SetModels(human, "acme", api.SetModelsInput{Default: row("opus", "")}); err != nil {
		t.Fatal(err)
	}
	noNotice(t, notices)

	after := f.get(t, "acme-2")
	if len(after.Moves) != len(before.Moves) || len(after.Edits) != len(before.Edits) || len(after.Comments) != len(before.Comments) {
		t.Errorf("set_models touched a Ticket: before %+v, after %+v", before, after)
	}
}

func TestModelsSurviveAReload(t *testing.T) {
	p, err := reloaded(t, func(f fixture) {
		f.project(t, "acme")
		f.setModels(t, api.SetModelsInput{Default: row("opus", ""), Chat: row("sonnet", "low"), Kinds: []api.KindModelsView{kind("map-research", "sonnet", "low"), kind("grilling", "", "max")}})
	}).core.GetProject(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if p.Models.Chat != row("sonnet", "low") {
		t.Errorf("chat row = %+v, want sonnet at low", p.Models.Chat)
	}
	wantModels(t, p.Models, row("opus", ""), kind("grilling", "", "max"), kind("map-research", "sonnet", "low"))
}

// guideLabels are Labels giving a Ticket the kind of work named guide.
func guideLabels(guide string) []string {
	switch {
	case guide == api.ImplementationStage:
		return nil
	case guide == domain.PrototypeLabel || guide == domain.RetroLabel:
		return []string{guide}
	case strings.HasPrefix(guide, "map-"):
		return []string{domain.WayfinderPrefix + strings.TrimPrefix(guide, "map-")}
	}
	return []string{domain.StagePrefix + guide}
}

// kindTicket puts a Ready Ticket of the kind of work named guide on Spec
// acme-1 as number n, with an unrelated Label too.
func (f fixture) kindTicket(t *testing.T, n int, guide string) {
	t.Helper()
	err := f.store.Update(func(w store.Writer) error {
		return w.PutTicket("acme", domain.Ticket{Number: n, Title: guide, State: domain.Ready, Spec: 1,
			Labels: append([]string{"ui"}, guideLabels(guide)...), Created: t0, Updated: t0})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTheModelChoiceTakesTheRowForTheTicketsKindOfWork(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Login")
	var kinds []api.KindModelsView
	for i, g := range core.GuideNames {
		f.kindTicket(t, 10+i, g)
		kinds = append(kinds, kind(g, "model-"+g, ""))
	}
	f.setModels(t, api.SetModelsInput{Default: row("opus", "high"), Kinds: kinds})
	for i, g := range core.GuideNames {
		want := choice(fromKind("model-"+g, g), set("high", api.SourceProjectDefault))
		if mc := f.modelChoice(t, core.DisplayID("acme", 10+i)); mc != want {
			t.Errorf("a %s Ticket's model choice = %+v, want %+v", g, mc, want)
		}
	}
}

func TestTheModelChoiceWorksOutEachValueByTheOrderOfPrecedence(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t) // acme-2, implementation

	if mc := f.modelChoice(t, "acme-2"); mc != choice(unset, unset) {
		t.Errorf("with nothing set, model choice = %+v", mc)
	}

	f.setModels(t, api.SetModelsInput{Default: row("opus", "")})
	if mc, want := f.modelChoice(t, "acme-2"), choice(set("opus", api.SourceProjectDefault), unset); mc != want {
		t.Errorf("with a default model, model choice = %+v, want %+v", mc, want)
	}

	// A row for another kind leaves this Ticket on the default.
	f.setModels(t, api.SetModelsInput{Default: row("opus", "medium"), Kinds: []api.KindModelsView{kind("spec", "sonnet", "low")}})
	if mc, want := f.modelChoice(t, "acme-2"), choice(set("opus", api.SourceProjectDefault), set("medium", api.SourceProjectDefault)); mc != want {
		t.Errorf("with another kind's row, model choice = %+v, want %+v", mc, want)
	}

	// The kind's row beats the default, value by value.
	f.setModels(t, api.SetModelsInput{Default: row("opus", "medium"), Kinds: []api.KindModelsView{kind("implementation", "", "xhigh")}})
	if mc, want := f.modelChoice(t, "acme-2"), choice(set("opus", api.SourceProjectDefault), fromKind("xhigh", "implementation")); mc != want {
		t.Errorf("with the kind's effort, model choice = %+v, want %+v", mc, want)
	}

	// A Tag beats them both.
	f.setTags(t, "acme-2", map[string]string{"effort": "low"})
	want := choice(set("opus", api.SourceProjectDefault), set("low", api.SourceTicketTag))
	if mc := f.modelChoice(t, "acme-2"); mc != want {
		t.Errorf("with an effort Tag, model choice = %+v, want %+v", mc, want)
	}
	if res := f.claim(t, api.ClaimInput{Ticket: "acme-2"}); res.ModelChoice == nil || *res.ModelChoice != want {
		t.Errorf("claim's model choice = %+v, want %+v", res.ModelChoice, want)
	}

	// A change after the Claim applies to the next turn.
	f.clock.Advance(time.Minute)
	f.setModels(t, api.SetModelsInput{Kinds: []api.KindModelsView{kind("implementation", "fable", "")}})
	if mc, want := f.modelChoice(t, "acme-2"), choice(fromKind("fable", "implementation"), set("low", api.SourceTicketTag)); mc != want {
		t.Errorf("after a change, model choice = %+v, want %+v", mc, want)
	}
}
