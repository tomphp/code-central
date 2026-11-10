package core_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

func (f fixture) setModels(t *testing.T, in core.SetModelsInput) core.ProjectView {
	t.Helper()
	in.Full = true
	w, err := f.core.SetModels(human, "acme", in)
	if err != nil {
		t.Fatalf("set_models: %v", err)
	}
	return *w.Record
}

// row is a Models row setting model and effort.
func row(model, effort string) core.ModelRowView {
	return core.ModelRowView{Model: model, Effort: effort}
}

func kind(name, model, effort string) core.KindModelsView {
	return core.KindModelsView{Kind: name, ModelRowView: row(model, effort)}
}

func choice(model core.ChoiceValue, effort core.ChoiceValue) core.ModelChoice {
	return core.ModelChoice{Model: model, Effort: effort}
}

func set(v string, s core.ChoiceSource) core.ChoiceValue {
	return core.ChoiceValue{Value: v, Source: s}
}

var unset = core.ChoiceValue{Source: core.SourceNone}

// wantModels checks a Project's Models lists the default row, then every
// kind of work in order, each row as want sets it.
func wantModels(t *testing.T, got core.ModelsView, def core.ModelRowView, want ...core.KindModelsView) {
	t.Helper()
	if got.Default != def {
		t.Errorf("default row = %+v, want %+v", got.Default, def)
	}
	var kinds []string
	for _, k := range got.Kinds {
		kinds = append(kinds, k.Kind)
		w := core.KindModelsView{Kind: k.Kind}
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
	if !slices.Equal(store.ModelKinds, core.GuideNames) {
		t.Fatalf("store.ModelKinds = %v, want core.GuideNames %v", store.ModelKinds, core.GuideNames)
	}
}

func TestSetModelsReplacesTheRowsWhole(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)
	p := f.setModels(t, core.SetModelsInput{Default: row(" opus ", "high"), Kinds: []core.KindModelsView{kind("implementation", "", "xhigh"), kind("grilling", "fable", "")}})
	wantModels(t, p.Models, row("opus", "high"), kind("grilling", "fable", ""), kind("implementation", "", "xhigh"))

	p = f.setModels(t, core.SetModelsInput{Kinds: []core.KindModelsView{kind("spec", "sonnet", ""), kind("prototype", "", "")}})
	wantModels(t, p.Models, row("", ""), kind("spec", "sonnet", ""))
}

func TestSetModelsIsTheHumans(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	for _, a := range []core.Actor{core.Agent, core.Integration} {
		_, err := f.core.SetModels(core.WithActor(ctx, a), "acme", core.SetModelsInput{Default: row("opus", "")})
		wantCode(t, err, core.ActionNotAllowed, "")
	}
	if p, _ := f.core.GetProject(ctx, "acme"); p.Models.Default != row("", "") {
		t.Fatalf("a refused set_models changed the Models: %+v", p.Models)
	}
}

func TestSetModelsValidatesBeforeWriting(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.setModels(t, core.SetModelsInput{Default: row("opus", "")})
	for _, c := range []struct {
		name  string
		in    core.SetModelsInput
		field string
		says  []string
	}{
		{"a row for something else", core.SetModelsInput{Kinds: []core.KindModelsView{kind("review", "opus", "")}}, "kinds", []string{`"review"`, strings.Join(core.GuideNames, ", ")}},
		{"a kind twice", core.SetModelsInput{Kinds: []core.KindModelsView{kind("spec", "opus", ""), kind("spec", "", "low")}}, "kinds", []string{`"spec"`}},
		{"a blank model", core.SetModelsInput{Default: row("  ", "")}, "default.model", []string{"blank"}},
		{"a multi-line model", core.SetModelsInput{Kinds: []core.KindModelsView{kind("tickets", "opus\nsonnet", "")}}, "kinds.tickets.model", []string{"one line"}},
		{"an effort outside the levels", core.SetModelsInput{Kinds: []core.KindModelsView{kind("implementation", "", "extreme")}}, "kinds.implementation.effort", []string{`"extreme"`, "low, medium, high, xhigh, max"}},
		{"a default effort outside the levels", core.SetModelsInput{Default: row("", "High")}, "default.effort", []string{`"High"`}},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := f.core.SetModels(human, "acme", c.in)
			wantCode(t, err, core.Invalid, c.field)
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

	w, err := f.core.SetModels(human, "acme", core.SetModelsInput{Default: row("opus", "")})
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
	if _, err := f.core.SetModels(human, "acme", core.SetModelsInput{Default: row("opus", "")}); err != nil {
		t.Fatal(err)
	}
	noNotice(t, notices)

	after := f.get(t, "acme-2")
	if len(after.Moves) != len(before.Moves) || len(after.Edits) != len(before.Edits) || len(after.Comments) != len(before.Comments) {
		t.Errorf("set_models touched a Ticket: before %+v, after %+v", before, after)
	}
}

func TestModelsSurviveAReload(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{core: core.New(s, clock.NewFake(t0), &ids.Sequence{}), clock: clock.NewFake(t0), store: s}
	f.project(t, "acme")
	f.setModels(t, core.SetModelsInput{Default: row("opus", ""), Kinds: []core.KindModelsView{kind("map-research", "sonnet", "low"), kind("grilling", "", "max")}})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := core.New(s, clock.NewFake(t0), &ids.Sequence{}).GetProject(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	wantModels(t, p.Models, row("opus", ""), kind("grilling", "", "max"), kind("map-research", "sonnet", "low"))
}

// guideLabels are Labels giving a Ticket the kind of work named guide.
func guideLabels(guide string) []string {
	switch {
	case guide == core.ImplementationStage:
		return nil
	case guide == core.PrototypeLabel:
		return []string{core.PrototypeLabel}
	case strings.HasPrefix(guide, "map-"):
		return []string{core.WayfinderPrefix + strings.TrimPrefix(guide, "map-")}
	}
	return []string{core.StagePrefix + guide}
}

// kindTicket puts a Ready Ticket of the kind of work named guide on Spec
// acme-1 as number n, with an unrelated Label too.
func (f fixture) kindTicket(t *testing.T, n int, guide string) {
	t.Helper()
	err := f.store.Update(func(w store.Writer) error {
		return w.PutTicket("acme", store.Ticket{Number: n, Title: guide, State: store.Ready, Spec: 1,
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
	var kinds []core.KindModelsView
	for i, g := range core.GuideNames {
		f.kindTicket(t, 10+i, g)
		kinds = append(kinds, kind(g, "model-"+g, ""))
	}
	f.setModels(t, core.SetModelsInput{Default: row("opus", "high"), Kinds: kinds})
	for i, g := range core.GuideNames {
		want := choice(set("model-"+g, core.SourceProjectKind), set("high", core.SourceProjectDefault))
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

	f.setModels(t, core.SetModelsInput{Default: row("opus", "")})
	if mc, want := f.modelChoice(t, "acme-2"), choice(set("opus", core.SourceProjectDefault), unset); mc != want {
		t.Errorf("with a default model, model choice = %+v, want %+v", mc, want)
	}

	// A row for another kind leaves this Ticket on the default.
	f.setModels(t, core.SetModelsInput{Default: row("opus", "medium"), Kinds: []core.KindModelsView{kind("spec", "sonnet", "low")}})
	if mc, want := f.modelChoice(t, "acme-2"), choice(set("opus", core.SourceProjectDefault), set("medium", core.SourceProjectDefault)); mc != want {
		t.Errorf("with another kind's row, model choice = %+v, want %+v", mc, want)
	}

	// The kind's row beats the default, value by value.
	f.setModels(t, core.SetModelsInput{Default: row("opus", "medium"), Kinds: []core.KindModelsView{kind("implementation", "", "xhigh")}})
	if mc, want := f.modelChoice(t, "acme-2"), choice(set("opus", core.SourceProjectDefault), set("xhigh", core.SourceProjectKind)); mc != want {
		t.Errorf("with the kind's effort, model choice = %+v, want %+v", mc, want)
	}

	// A Tag beats them both.
	f.setTags(t, "acme-2", map[string]string{"effort": "low"})
	want := choice(set("opus", core.SourceProjectDefault), set("low", core.SourceTicketTag))
	if mc := f.modelChoice(t, "acme-2"); mc != want {
		t.Errorf("with an effort Tag, model choice = %+v, want %+v", mc, want)
	}
	if res := f.claim(t, core.ClaimInput{Ticket: "acme-2"}); res.ModelChoice == nil || *res.ModelChoice != want {
		t.Errorf("claim's model choice = %+v, want %+v", res.ModelChoice, want)
	}

	// A change after the Claim applies to the next turn.
	f.clock.Advance(time.Minute)
	f.setModels(t, core.SetModelsInput{Kinds: []core.KindModelsView{kind("implementation", "fable", "")}})
	if mc, want := f.modelChoice(t, "acme-2"), choice(set("fable", core.SourceProjectKind), set("low", core.SourceTicketTag)); mc != want {
		t.Errorf("after a change, model choice = %+v, want %+v", mc, want)
	}
}
