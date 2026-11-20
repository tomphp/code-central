package core_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

func (f fixture) reportModels(t *testing.T, in api.ReportModelsInput) api.ProjectView {
	t.Helper()
	in.Full = true
	w, err := f.core.ReportModels(ctx, "acme", in)
	if err != nil {
		t.Fatalf("report_models: %v", err)
	}
	return *w.Record
}

// wantReported checks the Reported Models a Project read carries.
func wantReported(t *testing.T, got api.ReportedModelsView, want api.ReportedModelsView) {
	t.Helper()
	if !slices.Equal(got.EffortLevels, want.EffortLevels) || !slices.Equal(got.Aliases, want.Aliases) || !slices.Equal(got.ModelIDs, want.ModelIDs) ||
		got.ModelFlag != want.ModelFlag || got.EffortFlag != want.EffortFlag || !equalTimes(got.Reported, want.Reported) {
		t.Errorf("reported models = %+v, want %+v", got, want)
	}
}

func equalTimes(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func at(t time.Time) *time.Time { return &t }

var seeded = []string{"low", "medium", "high", "xhigh", "max"}

func TestBeforeAnyReportAProjectReadOffersTheSeededEffortLevelsAndNoModels(t *testing.T) {
	f := newFixture(t)
	p := f.project(t, "acme")
	wantReported(t, p.ReportedModels, api.ReportedModelsView{EffortLevels: seeded, Aliases: []string{}, ModelIDs: []string{}})
}

func TestReportModelsKeepsWhatTheFactoryFoundWithItsFlagsAndWhenItArrived(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.clock.Advance(time.Hour)
	p := f.reportModels(t, api.ReportModelsInput{EffortLevels: []string{" low ", "high", "low", ""}, Aliases: []string{"opus", "sonnet"}, ModelFlag: " sonnet ", EffortFlag: "high"})
	wantReported(t, p.ReportedModels, api.ReportedModelsView{EffortLevels: []string{"low", "high"}, Aliases: []string{"opus", "sonnet"}, ModelIDs: []string{},
		ModelFlag: "sonnet", EffortFlag: "high", Reported: at(t0.Add(time.Hour))})
}

func TestAnEmptyReportKeepsTheKeptLevelsAndAliasesButReplacesTheFlags(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	// Model IDs come from usage, never from a report.
	err := f.store.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.ReportedModels.ModelIDs = []string{"claude-opus-5-5"}
		return w.PutProject(p)
	})
	if err != nil {
		t.Fatal(err)
	}
	f.reportModels(t, api.ReportModelsInput{EffortLevels: []string{"low", "high"}, Aliases: []string{"opus"}, ModelFlag: "opus", EffortFlag: "low"})

	f.clock.Advance(time.Minute)
	p := f.reportModels(t, api.ReportModelsInput{})
	wantReported(t, p.ReportedModels, api.ReportedModelsView{EffortLevels: []string{"low", "high"}, Aliases: []string{"opus"}, ModelIDs: []string{"claude-opus-5-5"},
		Reported: at(t0.Add(time.Minute))})

	// Found again, they are replaced whole.
	p = f.reportModels(t, api.ReportModelsInput{EffortLevels: []string{"max"}, Aliases: []string{"fable"}})
	wantReported(t, p.ReportedModels, api.ReportedModelsView{EffortLevels: []string{"max"}, Aliases: []string{"fable"}, ModelIDs: []string{"claude-opus-5-5"},
		Reported: at(t0.Add(time.Minute))})
}

func TestReportModelsRefusesAValueSpanningLines(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	for _, c := range []struct {
		in    api.ReportModelsInput
		field string
	}{
		{api.ReportModelsInput{EffortLevels: []string{"low\nhigh"}}, "effort_levels"},
		{api.ReportModelsInput{Aliases: []string{"opus\r\nsonnet"}}, "aliases"},
		{api.ReportModelsInput{ModelFlag: "opus\nsonnet"}, "model_flag"},
		{api.ReportModelsInput{EffortFlag: "low\nhigh"}, "effort_flag"},
	} {
		_, err := f.core.ReportModels(ctx, "acme", c.in)
		wantCode(t, err, api.Invalid, c.field)
	}
	if p, _ := f.core.GetProject(ctx, "acme"); p.ReportedModels.Reported != nil {
		t.Errorf("a refused report was kept: %+v", p.ReportedModels)
	}
}

func TestOnceReportedTheValidEffortLevelsAreTheReportedOnes(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t) // acme-2
	f.setModels(t, api.SetModelsInput{Default: row("", "max")})
	f.setTags(t, "acme-2", map[string]string{"effort": "xhigh"})

	f.reportModels(t, api.ReportModelsInput{EffortLevels: []string{"low", "high", "turbo"}})
	// What was stored stays, though the report dropped it.
	if p, _ := f.core.GetProject(ctx, "acme"); p.Models.Default.Effort != "max" || !slices.Equal(p.ReportedModels.EffortLevels, []string{"low", "high", "turbo"}) {
		t.Errorf("after the report: models %+v, reported %+v", p.Models, p.ReportedModels)
	}
	if tk := f.get(t, "acme-2"); tk.Tags["effort"] != "xhigh" {
		t.Errorf("the report touched the Ticket's effort Tag: %v", tk.Tags)
	}

	_, err := f.core.SetModels(human, "acme", api.SetModelsInput{Default: row("", "max")})
	wantCode(t, err, api.Invalid, "default.effort")
	if !strings.Contains(err.Error(), "low, high, turbo") {
		t.Errorf("the refusal should list the reported levels: %v", err)
	}
	_, err = f.core.SetTags(ctx, "acme", api.SetTagsInput{Target: "acme-2", Tags: map[string]string{"effort": "medium"}})
	wantCode(t, err, api.Invalid, "tags")
	if !strings.Contains(err.Error(), "low, high, turbo") {
		t.Errorf("the refusal should list the reported levels: %v", err)
	}

	f.setModels(t, api.SetModelsInput{Default: row("", "turbo")})
	f.setTags(t, "acme-2", map[string]string{"effort": "turbo"})

	// A report that found no levels keeps the reported ones valid.
	f.reportModels(t, api.ReportModelsInput{})
	_, err = f.core.SetModels(human, "acme", api.SetModelsInput{Default: row("", "max")})
	wantCode(t, err, api.Invalid, "default.effort")
}

func TestReportModelsSendsTheProjectsNotice(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	notices, stop := f.core.Events().Subscribe()
	defer stop()

	w, err := f.core.ReportModels(ctx, "acme", api.ReportModelsInput{EffortLevels: []string{"low"}})
	if err != nil {
		t.Fatal(err)
	}
	if w.Record != nil || w.ID != "acme" || !slices.Equal(w.Changed, []string{"reported_models"}) {
		t.Errorf("acknowledgement = %+v, want reported_models changed on acme", w)
	}
	if got := <-notices; got != (events.Event{Type: events.ProjectChanged, Project: "acme"}) {
		t.Fatalf("got %+v, want the Project's notice", got)
	}
	noNotice(t, notices)
}

func TestReportedModelsSurviveAReload(t *testing.T) {
	c := reloaded(t, func(f fixture) {
		f.project(t, "acme")
		f.reportModels(t, api.ReportModelsInput{EffortLevels: []string{"low", "high"}, Aliases: []string{"opus", "sonnet"}, ModelFlag: "opus", EffortFlag: "high"})
	}).core
	p, err := c.GetProject(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	wantReported(t, p.ReportedModels, api.ReportedModelsView{EffortLevels: []string{"low", "high"}, Aliases: []string{"opus", "sonnet"}, ModelIDs: []string{},
		ModelFlag: "opus", EffortFlag: "high", Reported: at(t0)})
	// The reported levels still decide what is valid.
	_, err = c.SetModels(human, "acme", api.SetModelsInput{Default: row("", "max")})
	wantCode(t, err, api.Invalid, "default.effort")
}

// postModels posts one response per model as sess-1's usage.
func (f fixture) postModels(models ...string) {
	rs := make([]api.UsageResponse, len(models))
	for i, m := range models {
		rs[i] = api.UsageResponse{Kind: "implementation", Model: m}
	}
	f.core.PostUsage(ctx, "sess-1", rs)
}

func TestPostingUsageAddsEachUnseenModelIDToTheReportedModels(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)
	f.claim(t, api.ClaimInput{SessionID: "sess-1"})

	f.postModels("claude-opus-5-5", "<synthetic>", "", "claude-haiku-5-5", "claude-opus-5-5")
	f.postModels("claude-haiku-5-5", " claude-sonnet-5-5 ")
	p, _ := f.core.GetProject(ctx, "acme")
	if want := []string{"claude-opus-5-5", "claude-haiku-5-5", "claude-sonnet-5-5"}; !slices.Equal(p.ReportedModels.ModelIDs, want) {
		t.Errorf("model IDs = %v, want %v", p.ReportedModels.ModelIDs, want)
	}

	// A later report keeps them.
	p = f.reportModels(t, api.ReportModelsInput{EffortLevels: []string{"low"}, Aliases: []string{"opus"}})
	if want := []string{"claude-opus-5-5", "claude-haiku-5-5", "claude-sonnet-5-5"}; !slices.Equal(p.ReportedModels.ModelIDs, want) {
		t.Errorf("after report_models, model IDs = %v, want %v", p.ReportedModels.ModelIDs, want)
	}
}

func TestUsageFromAnUnknownSessionAddsNoModelIDs(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)
	f.core.PostUsage(ctx, "nobody", []api.UsageResponse{{Kind: "implementation", Model: "claude-opus-5-5"}})
	if p, _ := f.core.GetProject(ctx, "acme"); len(p.ReportedModels.ModelIDs) != 0 {
		t.Errorf("model IDs = %v, want none", p.ReportedModels.ModelIDs)
	}
}

func TestAnUnseenModelIDSendsTheProjectsNoticeAndASeenOneDoesNot(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t)
	f.claim(t, api.ClaimInput{SessionID: "sess-1"})
	notices, stop := f.core.Events().Subscribe()
	defer stop()

	f.postModels("claude-opus-5-5")
	if got := <-notices; got != (events.Event{Type: events.ProjectChanged, Project: "acme"}) {
		t.Fatalf("got %+v, want the Project's notice", got)
	}
	noNotice(t, notices)

	f.postModels("claude-opus-5-5", "<synthetic>")
	noNotice(t, notices)
}

func TestModelIDsFromUsageSurviveAReload(t *testing.T) {
	c := reloaded(t, func(f fixture) {
		f.readyTicket(t)
		f.claim(t, api.ClaimInput{SessionID: "sess-1"})
		f.postModels("claude-opus-5-5")
	}).core
	p, err := c.GetProject(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"claude-opus-5-5"}; !slices.Equal(p.ReportedModels.ModelIDs, want) {
		t.Errorf("model IDs = %v, want %v", p.ReportedModels.ModelIDs, want)
	}
}
