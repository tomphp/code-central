package core_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// The base the Factory reports under Pull Request (code-central-394):
// report_base, the Base diverged Focus Item, and the report lapsing with
// the Factory's heartbeat.

func (f fixture) pullRequestProject(t *testing.T) {
	t.Helper()
	f.project(t, "acme")
	if _, err := f.core.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: "git@github.com:acme/web.git", Base: "main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest}); err != nil {
		t.Fatal(err)
	}
	beat(f, t, "cc", "box", core.HeartbeatOnline, 10)
}

func (f fixture) reportBase(t *testing.T, in core.ReportBaseInput) {
	t.Helper()
	if _, err := f.core.ReportBase(ctx, "acme", in); err != nil {
		t.Fatal(err)
	}
}

func baseRow(t *testing.T, f fixture) (core.NeedsMeRow, bool) {
	t.Helper()
	v, err := f.core.NeedsMe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range v.Groups {
		if g.Reason == core.ReasonBaseDiverged {
			if len(g.Tickets) != 1 {
				t.Fatalf("group %+v, want one item per Project", g)
			}
			return g.Tickets[0], true
		}
	}
	return core.NeedsMeRow{}, false
}

func TestADivergedBaseIsOneNeedsMeItemWithItsCommits(t *testing.T) {
	f := newFixture(t)
	f.pullRequestProject(t)
	f.reportBase(t, core.ReportBaseInput{State: core.BaseDiverged, Local: "aaa111", Remote: "bbb222"})
	f.reportBase(t, core.ReportBaseInput{State: core.BaseDiverged, Local: "aaa111", Remote: "bbb222"})

	row, ok := baseRow(t, f)
	if !ok || row.Project != "acme" || row.ID != "" || row.BaseDiverged == nil ||
		row.BaseDiverged.Local != "aaa111" || row.BaseDiverged.Remote != "bbb222" || row.BaseDiverged.Base != "main" {
		t.Fatalf("row %+v ok=%v, want Base diverged with both commits", row, ok)
	}
	if v, _ := f.core.NeedsMe(ctx); v.Count != 1 {
		t.Fatalf("count %d, want 1", v.Count)
	}
	if fs := factoryStatus(f, t); fs.Base == nil || fs.Base.State != core.BaseDiverged {
		t.Fatalf("factory_status.base = %+v, want diverged", fs.Base)
	}
}

func TestABaseReportedCurrentClearsTheItem(t *testing.T) {
	f := newFixture(t)
	f.pullRequestProject(t)
	f.reportBase(t, core.ReportBaseInput{State: core.BaseDiverged, Local: "a", Remote: "b"})
	f.reportBase(t, core.ReportBaseInput{State: core.BaseCurrent, Local: "b", Remote: "b"})
	if _, ok := baseRow(t, f); ok {
		t.Fatal("Base diverged still shows once the base is current")
	}
}

func TestReportBaseRefusesAnUnknownStateOrMissingCommits(t *testing.T) {
	f := newFixture(t)
	f.pullRequestProject(t)
	for _, in := range []core.ReportBaseInput{
		{State: "sideways", Local: "a", Remote: "b"},
		{State: core.BaseDiverged, Local: "", Remote: "b"},
		{State: core.BaseDiverged, Local: "a", Remote: ""},
	} {
		if _, err := f.core.ReportBase(ctx, "acme", in); err == nil {
			t.Errorf("%+v accepted", in)
		}
	}
}

func TestTheBaseReportLapsesWithTheHeartbeat(t *testing.T) {
	f := newFixture(t)
	f.pullRequestProject(t)
	f.reportBase(t, core.ReportBaseInput{State: core.BaseDiverged, Local: "a", Remote: "b"})

	f.clock.Advance(time.Hour)
	if _, ok := baseRow(t, f); ok {
		t.Fatal("Base diverged outlived the Factory's heartbeat")
	}
	if fs := factoryStatus(f, t); fs.Base != nil {
		t.Fatalf("factory_status.base = %+v, want none", fs.Base)
	}

	// A Factory that comes back starts clean, whatever the old one said.
	beat(f, t, "cc", "box", core.HeartbeatOnline, 10)
	if _, ok := baseRow(t, f); ok {
		t.Fatal("a restarted Factory inherited the old base report")
	}
}

func TestAStoppingFactoryDropsTheBaseReport(t *testing.T) {
	f := newFixture(t)
	f.pullRequestProject(t)
	f.reportBase(t, core.ReportBaseInput{State: core.BaseDiverged, Local: "a", Remote: "b"})
	beat(f, t, "cc", "box", core.HeartbeatStopping, 10)
	if _, ok := baseRow(t, f); ok {
		t.Fatal("Base diverged outlived a stopped Factory")
	}
}

func TestBaseDivergedOnlyShowsWhereTheFactoryKeepsTheBaseLevel(t *testing.T) {
	f := newFixture(t)
	f.pullRequestProject(t)
	f.reportBase(t, core.ReportBaseInput{State: core.BaseDiverged, Local: "a", Remote: "b"})
	for _, in := range []core.SetLandingMethodInput{
		{Method: store.LandingMerge},
		{Method: store.LandingBranch},
	} {
		if _, err := f.core.SetLandingMethod(human, "acme", in); err != nil {
			t.Fatal(err)
		}
		if _, ok := baseRow(t, f); ok {
			t.Fatalf("Base diverged shows under %+v", in)
		}
	}
}

func TestBaseDivergedShowsUnderMergeWithPush(t *testing.T) {
	f := newFixture(t)
	f.pullRequestProject(t)
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingMerge, Push: true, Remote: "origin"}); err != nil {
		t.Fatal(err)
	}
	f.reportBase(t, core.ReportBaseInput{State: core.BaseDiverged, Local: "a", Remote: "b"})
	row, ok := baseRow(t, f)
	if !ok || row.BaseDiverged.Local != "a" || row.BaseDiverged.Remote != "b" {
		t.Fatalf("row %+v ok %v, want Base diverged with both commits", row, ok)
	}
	rows, err := f.core.ListProjects(ctx)
	if err != nil || len(rows) != 1 || rows[0].NeedsMe != 1 {
		t.Fatalf("rows %+v err %v, want needs_me 1", rows, err)
	}
}

func TestBaseDivergedCountsInTheProjectsNeedsMe(t *testing.T) {
	f := newFixture(t)
	f.pullRequestProject(t)
	f.reportBase(t, core.ReportBaseInput{State: core.BaseDiverged, Local: "a", Remote: "b"})
	rows, err := f.core.ListProjects(ctx)
	if err != nil || len(rows) != 1 || rows[0].NeedsMe != 1 {
		t.Fatalf("rows %+v err %v, want needs_me 1", rows, err)
	}
}

func missingRow(t *testing.T, f fixture) (core.NeedsMeRow, bool) {
	t.Helper()
	v, err := f.core.NeedsMe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range v.Groups {
		if g.Reason == core.ReasonBaseMissing {
			if len(g.Tickets) != 1 {
				t.Fatalf("group %+v, want one item per Project", g)
			}
			return g.Tickets[0], true
		}
	}
	return core.NeedsMeRow{}, false
}

func TestAMissingBaseShowsUnderEveryLandingMethod(t *testing.T) {
	for _, method := range []store.LandingMethod{store.LandingMerge, store.LandingBranch, store.LandingPullRequest} {
		f := newFixture(t)
		f.pullRequestProject(t)
		if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: method}); err != nil {
			t.Fatal(err)
		}
		// local and remote are ignored with missing.
		f.reportBase(t, core.ReportBaseInput{State: core.BaseMissing, Local: "ignored"})
		row, ok := missingRow(t, f)
		if !ok || row.Project != "acme" || row.ID != "" || row.BaseMissing == nil || row.BaseMissing.Base != "main" {
			t.Fatalf("%s: row %+v ok=%v, want Base missing naming main", method, row, ok)
		}
		if _, ok := baseRow(t, f); ok {
			t.Fatalf("%s: Base diverged shows beside Base missing", method)
		}
		if rows, _ := f.core.ListProjects(ctx); len(rows) != 1 || rows[0].NeedsMe != 1 {
			t.Fatalf("%s: rows %+v, want needs_me 1", method, rows)
		}
		f.reportBase(t, core.ReportBaseInput{State: core.BaseCurrent, Local: "a", Remote: "a"})
		if _, ok := missingRow(t, f); ok {
			t.Fatalf("%s: Base missing still shows once the base is current", method)
		}
	}
}

func TestBaseMissingLapsesWithTheHeartbeat(t *testing.T) {
	f := newFixture(t)
	f.pullRequestProject(t)
	f.reportBase(t, core.ReportBaseInput{State: core.BaseMissing})
	f.clock.Advance(time.Hour)
	if _, ok := missingRow(t, f); ok {
		t.Fatal("Base missing outlived the Factory's heartbeat")
	}
}

func TestBaseMissingStaysOffAnArchivedProject(t *testing.T) {
	f := newFixture(t)
	f.pullRequestProject(t)
	f.reportBase(t, core.ReportBaseInput{State: core.BaseMissing})
	if _, err := f.core.ArchiveProject(human, "acme"); err != nil {
		t.Skipf("cannot archive: %v", err)
	}
	if _, ok := missingRow(t, f); ok {
		t.Fatal("Base missing shows on an archived Project")
	}
}
