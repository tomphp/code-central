package core_test

import (
	"encoding/json"
	"maps"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// readyTicket is acme-2, a Ready implementation Ticket on Spec acme-1.
func (f fixture) readyTicket(t *testing.T) core.TicketView {
	t.Helper()
	return f.work(t, store.Ready)[0]
}

func (f fixture) setTags(t *testing.T, target string, tags map[string]string) {
	t.Helper()
	if _, err := f.core.SetTags(ctx, "acme", core.SetTagsInput{Target: core.Ref(target), Tags: tags}); err != nil {
		t.Fatalf("set_tags: %v", err)
	}
}

func (f fixture) modelChoice(t *testing.T, ticket string) core.ModelChoice {
	t.Helper()
	mc, err := f.core.GetModelChoice(ctx, "acme", core.GetModelChoiceInput{Ticket: core.Ref(ticket)})
	if err != nil {
		t.Fatalf("get_model_choice: %v", err)
	}
	return mc
}

// none is a Model Choice that sets neither model nor effort.
var none = core.ModelChoice{Model: core.ChoiceValue{Source: core.SourceNone}, Effort: core.ChoiceValue{Source: core.SourceNone}}

func TestWithNoTagsTheModelChoiceSetsNothing(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)
	if mc := f.modelChoice(t, "acme-2"); mc != none {
		t.Errorf("model choice = %+v, want nothing set", mc)
	}
	if res := f.claim(t, core.ClaimInput{Ticket: "acme-2"}); res.ModelChoice == nil || *res.ModelChoice != none {
		t.Errorf("claim's model choice = %+v, want nothing set", res.ModelChoice)
	}
}

func TestATicketsModelAndEffortTagsSetTheModelChoiceEachOnItsOwn(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)

	f.setTags(t, "acme-2", map[string]string{"model": "opus"})
	want := core.ModelChoice{Model: core.ChoiceValue{Value: "opus", Source: core.SourceTicketTag}, Effort: core.ChoiceValue{Source: core.SourceNone}}
	if mc := f.modelChoice(t, "acme-2"); mc != want {
		t.Errorf("with a model Tag, model choice = %+v, want %+v", mc, want)
	}

	if _, err := f.core.RemoveTags(ctx, "acme", core.RemoveTagsInput{Target: "acme-2", Keys: []string{"model"}}); err != nil {
		t.Fatal(err)
	}
	f.setTags(t, "acme-2", map[string]string{"effort": "high"})
	want = core.ModelChoice{Model: core.ChoiceValue{Source: core.SourceNone}, Effort: core.ChoiceValue{Value: "high", Source: core.SourceTicketTag}}
	if mc := f.modelChoice(t, "acme-2"); mc != want {
		t.Errorf("with an effort Tag, model choice = %+v, want %+v", mc, want)
	}

	f.setTags(t, "acme-2", map[string]string{"model": "claude-opus-5-5"})
	want.Model = core.ChoiceValue{Value: "claude-opus-5-5", Source: core.SourceTicketTag}
	res := f.claim(t, core.ClaimInput{Ticket: "acme-2"})
	if res.ModelChoice == nil || *res.ModelChoice != want {
		t.Errorf("claim's model choice = %+v, want %+v", res.ModelChoice, want)
	}
}

func TestATagChangedAfterTheClaimAppliesToTheNextTurn(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)
	f.claim(t, core.ClaimInput{Ticket: "acme-2"})
	f.setTags(t, "acme-2", map[string]string{"model": "fable", "effort": "max"})
	want := core.ModelChoice{Model: core.ChoiceValue{Value: "fable", Source: core.SourceTicketTag}, Effort: core.ChoiceValue{Value: "max", Source: core.SourceTicketTag}}
	if mc := f.modelChoice(t, "acme-2"); mc != want {
		t.Errorf("model choice = %+v, want %+v", mc, want)
	}
}

func TestSetTagsRejectsAnEffortOutsideTheValidLevelsListingThem(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)
	_, err := f.core.SetTags(ctx, "acme", core.SetTagsInput{Target: "acme-2", Tags: map[string]string{"effort": "extreme"}})
	wantCode(t, err, core.Invalid, "tags")
	if !strings.Contains(err.Error(), "low, medium, high, xhigh, max") {
		t.Errorf("error %q doesn't list the valid levels", err)
	}
	if tags := f.get(t, "acme-2").Tags; len(tags) != 0 {
		t.Errorf("tags = %v, want nothing written", tags)
	}
	for _, level := range []string{"low", "medium", "high", "xhigh", "max"} {
		f.setTags(t, "acme-2", map[string]string{"effort": level})
	}
	// model takes any single-line value.
	f.setTags(t, "acme-2", map[string]string{"model": "some-future-model[1m]"})
}

func TestModelAndEffortTagsOnASpecAreOrdinaryTags(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)
	f.setTags(t, "acme-1", map[string]string{"model": "opus", "effort": "anything"})
	if mc := f.modelChoice(t, "acme-2"); mc != none {
		t.Errorf("model choice = %+v, want the Spec's Tags to have no effect", mc)
	}
}

func TestAgentsGetTicketLeavesTheModelChoiceOut(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)
	f.setTags(t, "acme-2", map[string]string{"model": "opus"})
	b, err := json.Marshal(f.get(t, "acme-2"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "model_choice") {
		t.Errorf("get_ticket = %s, want no model_choice", b)
	}
}

func TestTheUIsTicketReadCarriesTheModelChoiceWithSources(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)
	read := func() *core.ModelChoice {
		t.Helper()
		v, err := f.core.GetTicket(ctx, "acme", core.GetTicketInput{Ticket: "acme-2", WithModelChoice: true})
		if err != nil {
			t.Fatalf("get_ticket: %v", err)
		}
		return v.ModelChoice
	}
	if mc := read(); mc == nil || *mc != none {
		t.Errorf("with nothing set, model choice = %+v, want nothing set", mc)
	}
	f.setModels(t, core.SetModelsInput{Default: row("sonnet", "low"), Kinds: []core.KindModelsView{kind("implementation", "opus", "")}})
	if mc, want := read(), choice(fromKind("opus", "implementation"), set("low", core.SourceProjectDefault)); mc == nil || *mc != want {
		t.Errorf("with Models rows, model choice = %+v, want %+v", mc, want)
	}
	f.setTags(t, "acme-2", map[string]string{"effort": "max"})
	if mc, want := read(), choice(fromKind("opus", "implementation"), set("max", core.SourceTicketTag)); mc == nil || *mc != want {
		t.Errorf("with an effort Tag, model choice = %+v, want %+v", mc, want)
	}
}

func TestReportModelChoiceRecordsTheLatestTurnOnTheClaim(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)
	res := f.claim(t, core.ClaimInput{Ticket: "acme-2"})
	report := func(model, effort string) core.ClaimView {
		t.Helper()
		tk, err := record(f.core.ReportModelChoice(ctx, "acme", core.ReportModelChoiceInput{Ticket: "acme-2", ClaimID: res.ClaimID, Model: model, Effort: effort, Full: true}))
		if err != nil {
			t.Fatalf("report_model_choice: %v", err)
		}
		return tk.Claims[0]
	}
	if cl := report("opus", "high"); cl.Model != "opus" || cl.Effort != "high" {
		t.Errorf("claim = %+v, want opus at high", cl)
	}
	// The next turn's overwrites it; empty is Claude Code's default.
	if cl := report("", "low"); cl.Model != "" || cl.Effort != "low" {
		t.Errorf("claim = %+v, want the default model at low", cl)
	}
	if cl := f.get(t, "acme-2").Claims[0]; cl.Model != "" || cl.Effort != "low" {
		t.Errorf("get_ticket's claim = %+v, want the default model at low", cl)
	}
}

func TestReportModelChoiceOnAnEndedClaimIsStale(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)
	res := f.claim(t, core.ClaimInput{Ticket: "acme-2"})
	if _, err := f.core.Release(human, "acme", core.ReleaseInput{Ticket: "acme-2"}); err != nil {
		t.Fatal(err)
	}
	_, err := f.core.ReportModelChoice(ctx, "acme", core.ReportModelChoiceInput{Ticket: "acme-2", ClaimID: res.ClaimID, Model: "opus"})
	wantCode(t, err, core.StaleClaim, "")
	if cl := f.get(t, "acme-2").Claims[0]; cl.Model != "" {
		t.Errorf("claim = %+v, want nothing recorded", cl)
	}
}

func TestAClaimsModelAndEffortSurviveAReload(t *testing.T) {
	again := reloaded(t, func(f fixture) {
		f.readyTicket(t)
		res := f.claim(t, core.ClaimInput{Ticket: "acme-2"})
		if _, err := f.core.ReportModelChoice(ctx, "acme", core.ReportModelChoiceInput{Ticket: "acme-2", ClaimID: res.ClaimID, Model: "opus", Effort: "xhigh"}); err != nil {
			t.Fatal(err)
		}
	}).core
	tk, err := again.GetTicket(ctx, "acme", whole("acme-2"))
	if err != nil {
		t.Fatal(err)
	}
	if cl := tk.Claims[0]; cl.Model != "opus" || cl.Effort != "xhigh" {
		t.Errorf("claim after a reload = %+v, want opus at xhigh", cl)
	}
}

func TestCreateTicketTakesTagsFromTheStart(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)
	tags := map[string]string{"model": "claude-opus-5-5", "effort": "high", "area": "web"}
	if _, err := f.core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "tagged", Tags: tags}); err != nil {
		t.Fatalf("create_ticket: %v", err)
	}
	if got := f.get(t, "acme-3").Tags; !maps.Equal(got, tags) {
		t.Errorf("tags = %v, want %v", got, tags)
	}
	rows, err := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{Tags: map[string]string{"effort": "high"}})
	if err != nil || len(rows) != 1 || rows[0].ID != "acme-3" {
		t.Errorf("find by Tag = %+v, %v", rows, err)
	}
	want := core.ModelChoice{
		Model:  core.ChoiceValue{Value: "claude-opus-5-5", Source: core.SourceTicketTag},
		Effort: core.ChoiceValue{Value: "high", Source: core.SourceTicketTag},
	}
	if mc := f.modelChoice(t, "acme-3"); mc != want {
		t.Errorf("model choice = %+v, want %+v", mc, want)
	}
}

func TestCreateTicketRejectsBadTagsAndCreatesNothing(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)
	for name, tags := range map[string]map[string]string{
		"effort":     {"effort": "extreme"},
		"bad key":    {"Bad Key": "x"},
		"empty":      {"area": " "},
		"multi-line": {"area": "a\nb"},
	} {
		_, err := f.core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "t", Tags: tags})
		wantCode(t, err, core.Invalid, "tags")
		if err != nil && name == "effort" && !strings.Contains(err.Error(), "low, medium, high, xhigh, max") {
			t.Errorf("error %q doesn't list the valid levels", err)
		}
	}
	rows, err := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{})
	if err != nil || len(rows) != 1 {
		t.Errorf("tickets = %+v, %v; want only the first", rows, err)
	}
}

func TestCreateTicketWithEmptyTagsIsAsBefore(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)
	for _, tags := range []map[string]string{nil, {}} {
		if _, err := f.core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "plain", Tags: tags}); err != nil {
			t.Fatalf("create_ticket: %v", err)
		}
	}
	if got := f.get(t, "acme-3").Tags; len(got) != 0 {
		t.Errorf("tags = %v, want none", got)
	}
}

func TestTheUIsTicketReadCarriesTheAutoAcceptKindAndTheMCPReadDoesNot(t *testing.T) {
	f := specMergeWorld(t) // acme-1 with an implementation Ticket, acme-2
	for _, stage := range []string{"grilling", "spec", "tickets"} {
		f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: stage, Labels: []string{core.StagePrefix + stage}}) // acme-3 to acme-5
	}
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "Try", Labels: []string{core.PrototypeLabel}}) // acme-6
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "Chart", Labels: []string{"wayfinder:chart"}}) // acme-7
	merge := f.openSpecMerge(t, "README").Ticket
	m, err := f.core.SubmitMap(ctx, "acme", core.SubmitMapInput{Text: "# Offline mode\n\nbody\n"})
	if err != nil {
		t.Fatalf("submit_map: %v", err)
	}
	mapTicket := f.ticket(t, "acme", core.CreateTicketInput{Spec: core.Ref(m.Spec.ID), Title: "In the map"}).ID

	for ticket, want := range map[string]store.AutoAcceptKind{
		"acme-2": "implementation", "acme-3": "grilling", "acme-4": "spec", "acme-5": "tickets",
		merge: "merge", "acme-6": "", "acme-7": "", mapTicket: "",
	} {
		v, err := f.core.GetTicket(ctx, "acme", core.GetTicketInput{Ticket: core.Ref(ticket), WithModelChoice: true})
		if err != nil {
			t.Fatalf("get_ticket %s: %v", ticket, err)
		}
		if v.AutoAcceptKind != want {
			t.Errorf("%s: auto_accept_kind = %q, want %q", ticket, v.AutoAcceptKind, want)
		}
		plain, err := f.core.GetTicket(ctx, "acme", core.GetTicketInput{Ticket: core.Ref(ticket)})
		if err != nil {
			t.Fatal(err)
		}
		if plain.AutoAcceptKind != "" {
			t.Errorf("%s: the MCP read carries auto_accept_kind %q", ticket, plain.AutoAcceptKind)
		}
	}
}
