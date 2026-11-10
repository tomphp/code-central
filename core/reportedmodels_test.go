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

func (f fixture) reportModels(t *testing.T, in core.ReportModelsInput) core.ProjectView {
	t.Helper()
	in.Full = true
	w, err := f.core.ReportModels(ctx, "acme", in)
	if err != nil {
		t.Fatalf("report_models: %v", err)
	}
	return *w.Record
}

// wantReported checks the Reported Models a Project read carries.
func wantReported(t *testing.T, got core.ReportedModelsView, want core.ReportedModelsView) {
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
	wantReported(t, p.ReportedModels, core.ReportedModelsView{EffortLevels: seeded, Aliases: []string{}, ModelIDs: []string{}})
}

func TestReportModelsKeepsWhatTheFactoryFoundWithItsFlagsAndWhenItArrived(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.clock.Advance(time.Hour)
	p := f.reportModels(t, core.ReportModelsInput{EffortLevels: []string{" low ", "high", "low", ""}, Aliases: []string{"opus", "sonnet"}, ModelFlag: " sonnet ", EffortFlag: "high"})
	wantReported(t, p.ReportedModels, core.ReportedModelsView{EffortLevels: []string{"low", "high"}, Aliases: []string{"opus", "sonnet"}, ModelIDs: []string{},
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
	f.reportModels(t, core.ReportModelsInput{EffortLevels: []string{"low", "high"}, Aliases: []string{"opus"}, ModelFlag: "opus", EffortFlag: "low"})

	f.clock.Advance(time.Minute)
	p := f.reportModels(t, core.ReportModelsInput{})
	wantReported(t, p.ReportedModels, core.ReportedModelsView{EffortLevels: []string{"low", "high"}, Aliases: []string{"opus"}, ModelIDs: []string{"claude-opus-5-5"},
		Reported: at(t0.Add(time.Minute))})

	// Found again, they are replaced whole.
	p = f.reportModels(t, core.ReportModelsInput{EffortLevels: []string{"max"}, Aliases: []string{"fable"}})
	wantReported(t, p.ReportedModels, core.ReportedModelsView{EffortLevels: []string{"max"}, Aliases: []string{"fable"}, ModelIDs: []string{"claude-opus-5-5"},
		Reported: at(t0.Add(time.Minute))})
}

func TestReportModelsRefusesAValueSpanningLines(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	for _, c := range []struct {
		in    core.ReportModelsInput
		field string
	}{
		{core.ReportModelsInput{EffortLevels: []string{"low\nhigh"}}, "effort_levels"},
		{core.ReportModelsInput{Aliases: []string{"opus\r\nsonnet"}}, "aliases"},
		{core.ReportModelsInput{ModelFlag: "opus\nsonnet"}, "model_flag"},
		{core.ReportModelsInput{EffortFlag: "low\nhigh"}, "effort_flag"},
	} {
		_, err := f.core.ReportModels(ctx, "acme", c.in)
		wantCode(t, err, core.Invalid, c.field)
	}
	if p, _ := f.core.GetProject(ctx, "acme"); p.ReportedModels.Reported != nil {
		t.Errorf("a refused report was kept: %+v", p.ReportedModels)
	}
}

func TestOnceReportedTheValidEffortLevelsAreTheReportedOnes(t *testing.T) {
	f := newFixture(t)
	f.readyTicket(t) // acme-2
	f.setModels(t, core.SetModelsInput{Default: row("", "max")})
	f.setTags(t, "acme-2", map[string]string{"effort": "xhigh"})

	f.reportModels(t, core.ReportModelsInput{EffortLevels: []string{"low", "high", "turbo"}})
	// What was stored stays, though the report dropped it.
	if p, _ := f.core.GetProject(ctx, "acme"); p.Models.Default.Effort != "max" || !slices.Equal(p.ReportedModels.EffortLevels, []string{"low", "high", "turbo"}) {
		t.Errorf("after the report: models %+v, reported %+v", p.Models, p.ReportedModels)
	}
	if tk := f.get(t, "acme-2"); tk.Tags["effort"] != "xhigh" {
		t.Errorf("the report touched the Ticket's effort Tag: %v", tk.Tags)
	}

	_, err := f.core.SetModels(human, "acme", core.SetModelsInput{Default: row("", "max")})
	wantCode(t, err, core.Invalid, "default.effort")
	if !strings.Contains(err.Error(), "low, high, turbo") {
		t.Errorf("the refusal should list the reported levels: %v", err)
	}
	_, err = f.core.SetTags(ctx, "acme", core.SetTagsInput{Target: "acme-2", Tags: map[string]string{"effort": "medium"}})
	wantCode(t, err, core.Invalid, "tags")
	if !strings.Contains(err.Error(), "low, high, turbo") {
		t.Errorf("the refusal should list the reported levels: %v", err)
	}

	f.setModels(t, core.SetModelsInput{Default: row("", "turbo")})
	f.setTags(t, "acme-2", map[string]string{"effort": "turbo"})

	// A report that found no levels keeps the reported ones valid.
	f.reportModels(t, core.ReportModelsInput{})
	_, err = f.core.SetModels(human, "acme", core.SetModelsInput{Default: row("", "max")})
	wantCode(t, err, core.Invalid, "default.effort")
}

func TestReportModelsSendsTheProjectsNotice(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	notices, stop := f.core.Events().Subscribe()
	defer stop()

	w, err := f.core.ReportModels(ctx, "acme", core.ReportModelsInput{EffortLevels: []string{"low"}})
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
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{core: core.New(s, clock.NewFake(t0), &ids.Sequence{}), clock: clock.NewFake(t0), store: s}
	f.project(t, "acme")
	f.reportModels(t, core.ReportModelsInput{EffortLevels: []string{"low", "high"}, Aliases: []string{"opus", "sonnet"}, ModelFlag: "opus", EffortFlag: "high"})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := core.New(s, clock.NewFake(t0), &ids.Sequence{})
	p, err := c.GetProject(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	wantReported(t, p.ReportedModels, core.ReportedModelsView{EffortLevels: []string{"low", "high"}, Aliases: []string{"opus", "sonnet"}, ModelIDs: []string{},
		ModelFlag: "opus", EffortFlag: "high", Reported: at(t0)})
	// The reported levels still decide what is valid.
	_, err = c.SetModels(human, "acme", core.SetModelsInput{Default: row("", "max")})
	wantCode(t, err, core.Invalid, "default.effort")
}
