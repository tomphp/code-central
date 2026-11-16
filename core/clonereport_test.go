package core_test

import (
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
)

// The Factory's clone report (Spec code-central-848): report_repositories,
// each Repository's clone status in the Project's Repositories, the
// Repository missing Focus Item, and the report lapsing with the
// Factory's heartbeat.

// webRepository is the Repository the clone report tests add, beside the
// Home Repository acme.
var webRepository = core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"}

// twoRepositories is Project acme with Repository web added and a
// Factory online.
func twoRepositories(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t)
	f.project(t, "acme")
	f.addRepository(t, webRepository)
	beat(f, t, "cc", "box", core.HeartbeatOnline, 10)
	return f
}

func (f fixture) reportRepositories(t *testing.T, rs ...core.RepositoryReport) {
	t.Helper()
	if _, err := f.core.ReportRepositories(ctx, "acme", core.ReportRepositoriesInput{Repositories: rs}); err != nil {
		t.Fatal(err)
	}
}

// missingRows are the Repository missing rows in Needs Me.
func missingRows(t *testing.T, f fixture) []core.NeedsMeRow {
	t.Helper()
	var out []core.NeedsMeRow
	for _, row := range f.needsMe(t) {
		if row.Reason == core.ReasonRepositoryMissing {
			out = append(out, row)
		}
	}
	return out
}

var (
	homeReady  = core.RepositoryReport{Name: "acme", Path: "/src/acme", State: core.CloneReady}
	webReady   = core.RepositoryReport{Name: "web", Path: "/src/web", State: core.CloneReady}
	webMissing = core.RepositoryReport{Name: "web", Path: "/src/web", State: core.CloneMissing, Problem: "/src/web isn't a git repository"}
)

func TestEachRepositoryShowsItsReportedCloneStatusAndPath(t *testing.T) {
	f := twoRepositories(t)
	if c := repository(t, f.projectView(t, "acme"), "web").Clone; c != nil {
		t.Fatalf("clone %+v before any report, want none", c)
	}

	f.reportRepositories(t, homeReady, webMissing)
	p := f.projectView(t, "acme")
	home, web := repository(t, p, "acme").Clone, repository(t, p, "web").Clone
	if home == nil || home.State != core.CloneReady || home.Path != "/src/acme" {
		t.Fatalf("Home clone %+v, want ready at /src/acme", home)
	}
	if web == nil || web.State != core.CloneMissing || web.Path != "/src/web" || web.Problem != webMissing.Problem {
		t.Fatalf("web clone %+v, want missing at /src/web with its problem", web)
	}
}

func TestAMissingRepositoryIsAFocusItemNamingItsPathCloneURLProblemAndFix(t *testing.T) {
	f := twoRepositories(t)
	f.reportRepositories(t, homeReady, webMissing)

	rows := missingRows(t, f)
	if len(rows) != 1 {
		t.Fatalf("rows %+v, want one Repository missing item", rows)
	}
	row := rows[0]
	m := row.RepositoryMissing
	if row.Project != "acme" || row.ID != "" || !row.FocusItem || m == nil {
		t.Fatalf("row %+v, want a Focus Item of acme's with no Ticket", row)
	}
	if m.Repository != "web" || m.Path != "/src/web" || m.CloneURL != webRepository.CloneURL || m.Problem != webMissing.Problem {
		t.Fatalf("detail %+v, want web's path, Clone URL and problem", m)
	}
	if !strings.Contains(m.Fix, "local path") {
		t.Fatalf("fix %q, want it to offer setting a local path", m.Fix)
	}
	if v, _ := f.core.NeedsMe(ctx); v.Count != 1 {
		t.Fatalf("count %d, want 1", v.Count)
	}
	if rows, _ := f.core.ListProjects(ctx); rows[0].NeedsMe != 1 {
		t.Fatalf("list_projects needs_me %d, want 1", rows[0].NeedsMe)
	}
}

func TestAMissingRepositoryKeepsWaitingSinceItWasFirstReported(t *testing.T) {
	f := twoRepositories(t)
	f.reportRepositories(t, homeReady, webMissing)
	since := missingRows(t, f)[0].Since
	f.clock.Advance(5 * time.Second)
	beat(f, t, "cc", "box", core.HeartbeatOnline, 10)
	f.reportRepositories(t, homeReady, webMissing)
	if got := missingRows(t, f)[0].Since; !got.Equal(since) {
		t.Fatalf("since %v, want the first report's %v", got, since)
	}
}

func TestARepositoryReportedReadyClearsTheItem(t *testing.T) {
	f := twoRepositories(t)
	f.reportRepositories(t, homeReady, webMissing)
	f.reportRepositories(t, homeReady, webReady)
	if rows := missingRows(t, f); len(rows) != 0 {
		t.Fatalf("rows %+v once web is ready, want none", rows)
	}
}

func TestARemovedRepositoryIsNoLongerMissing(t *testing.T) {
	f := twoRepositories(t)
	f.reportRepositories(t, homeReady, webMissing)
	if _, err := f.core.RemoveRepository(human, "acme", core.RemoveRepositoryInput{Name: "web"}); err != nil {
		t.Fatal(err)
	}
	if rows := missingRows(t, f); len(rows) != 0 {
		t.Fatalf("rows %+v for a removed Repository, want none", rows)
	}
}

func TestTheCloneReportLapsesWithTheHeartbeat(t *testing.T) {
	f := twoRepositories(t)
	f.reportRepositories(t, homeReady, webMissing)

	f.clock.Advance(time.Hour)
	if rows := missingRows(t, f); len(rows) != 0 {
		t.Fatal("Repository missing outlived the Factory's heartbeat")
	}
	if c := repository(t, f.projectView(t, "acme"), "web").Clone; c != nil {
		t.Fatalf("clone %+v with no Factory online, want none", c)
	}

	// A Factory that comes back starts clean, whatever the old one said.
	beat(f, t, "cc", "box", core.HeartbeatOnline, 10)
	if rows := missingRows(t, f); len(rows) != 0 {
		t.Fatal("a restarted Factory inherited the old clone report")
	}
}

func TestAStoppingFactoryDropsTheCloneReport(t *testing.T) {
	f := twoRepositories(t)
	f.reportRepositories(t, homeReady, webMissing)
	beat(f, t, "cc", "box", core.HeartbeatStopping, 10)
	if rows := missingRows(t, f); len(rows) != 0 {
		t.Fatal("Repository missing outlived a stopped Factory")
	}
}

func TestReportRepositoriesRefusesWhatIsntACloneReport(t *testing.T) {
	f := twoRepositories(t)
	for _, in := range [][]core.RepositoryReport{
		{{Name: "", Path: "/src/web", State: core.CloneReady}},
		{{Name: "web", Path: "", State: core.CloneReady}},
		{{Name: "web", Path: "/src/web", State: "sideways"}},
		{{Name: "web", Path: "/src/web", State: core.CloneMissing}},
		{webReady, webReady},
	} {
		if _, err := f.core.ReportRepositories(ctx, "acme", core.ReportRepositoriesInput{Repositories: in}); err == nil {
			t.Errorf("%+v accepted", in)
		}
	}
}
