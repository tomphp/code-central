// Azure Pipelines runs: the CI Watcher lists builds and timelines through
// the fake Azure DevOps API, reports them against a Ticket or a CI Rule,
// and a failing build on a rule's branch opens a Fix Ticket naming its
// failed jobs. Observed only through the HTTP API.

package ciwatcher_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/ciwatcher"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/internal/fakeazdo"
	"github.com/tomoram/code-central/store"
)

const buildsPath = fakeazdo.ProjectPath + "/_apis/build/builds"

// azureWatching is a Project whose CI Provider is Azure Pipelines, over a
// fake Azure DevOps.
type azureWatching struct {
	*watching
	azdo *fakeazdo.Server
}

func newAzureWatching(t *testing.T) *azureWatching {
	t.Helper()
	a := &azureWatching{watching: newWatching(t), azdo: fakeazdo.New(t)}
	a.track()
	return a
}

// track turns CI Tracking on for the fake, ignoring ignored pipelines.
func (a *azureWatching) track(ignored ...string) {
	a.t.Helper()
	human := core.WithActor(context.Background(), core.Human)
	must(a.t, func() error {
		_, err := a.srv.Core.SetCITracking(human, "acme", core.SetCITrackingInput{Tracking: true, Provider: store.CIAzurePipelines,
			ProjectURL: a.azdo.ProjectURL(), IgnoredWorkflows: ignored})
		return err
	})
}

// startAzure runs the loop with the PAT in its environment.
func (a *azureWatching) startAzure() {
	a.t.Helper()
	r, pw := io.Pipe()
	a.stdin, a.done = pw, make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	a.t.Cleanup(func() {
		cancel()
		pw.Close()
	})
	go func() {
		a.done <- ciwatcher.Watch(ctx, ciwatcher.Config{
			Client: a.srv.Client, Project: "acme", Clock: a.srv.Clock, Stdin: r, Log: io.Discard,
			Env: gh.Env{Environ: []string{"ADO_PAT=" + secretPAT}},
		})
	}()
}

// azBuild is one build as Azure DevOps lists it.
type azBuild struct {
	id                int
	pipeline          string
	branch, commit    string
	status, result    string
	changed, finished string
}

func (b azBuild) json() string {
	finish, result := "", ""
	if b.finished != "" {
		finish = fmt.Sprintf(`,"finishTime":%q`, b.finished)
	}
	if b.result != "" {
		result = fmt.Sprintf(`,"result":%q`, b.result)
	}
	if b.branch == "" {
		b.branch = "refs/heads/main"
	}
	return fmt.Sprintf(`{"id":%d,"buildNumber":"20260105.%d","status":%q%s,"sourceBranch":%q,"sourceVersion":%q,"startTime":"2026-01-05T09:01:00Z"%s,`+
		`"lastChangedDate":%q,"definition":{"name":%q,"path":"\\folder"},"_links":{"web":{"href":"https://dev.azure.com/acme/web/_build/results?buildId=%d"}}}`,
		b.id, b.id, b.status, result, b.branch, b.commit, finish, b.changed, b.pipeline, b.id)
}

// builds scripts the list of builds. The fake matches a path by prefix and
// the latest answer wins, so script a build's timeline after it.
func (a *azureWatching) builds(builds ...azBuild) {
	var items []string
	for _, b := range builds {
		items = append(items, b.json())
	}
	a.azdo.Script(fakeazdo.Answer{Method: "GET", Path: buildsPath, Body: `{"count":` + fmt.Sprint(len(items)) + `,"value":[` + strings.Join(items, ",") + `]}`})
}

// azRecord is one timeline record: its type, name, result ("" while it
// runs) and attempt.
type azRecord struct {
	typ, name, result string
	attempt           int
}

// timeline scripts a build's timeline.
func (a *azureWatching) timeline(id int, records ...azRecord) {
	var items []string
	for i, r := range records {
		state, result := "completed", fmt.Sprintf("%q", r.result)
		if r.result == "" {
			state, result = "inProgress", "null"
		}
		items = append(items, fmt.Sprintf(`{"id":"rec%d","parentId":"p","type":%q,"name":%q,"state":%q,"result":%s,"attempt":%d}`,
			i, r.typ, r.name, state, result, r.attempt))
	}
	a.azdo.Script(fakeazdo.Answer{Method: "GET", Path: fmt.Sprintf("%s/%d/timeline", buildsPath, id), Body: `{"records":[` + strings.Join(items, ",") + `]}`})
}

// requests counts the requests whose path holds part.
func (a *azureWatching) requests(part string) int {
	n := 0
	for _, r := range a.azdo.Requests() {
		if strings.Contains(r.Path, part) {
			n++
		}
	}
	return n
}

func (a *azureWatching) commitLists() int {
	n := 0
	for _, r := range a.azdo.Requests() {
		if r.Path == buildsPath && len(r.Query["$top"]) > 0 && r.Query["$top"][0] == "200" {
			n++
		}
	}
	return n
}

// poll waits for the loop to wait out its interval while a pushed commit
// waits, then to list again and wait.
func (a *azureWatching) poll() {
	a.t.Helper()
	n := a.commitLists()
	a.srv.Clock.BlockUntil(1)
	a.srv.Clock.Advance(ciwatcher.WaitingInterval)
	a.waitFor("a poll after the waiting interval", func() bool { return a.commitLists() > n })
	a.srv.Clock.BlockUntil(1)
}

func TestTheCIWatcherReportsAnAzureCommitsBuildsAsRunsMatchedByCommit(t *testing.T) {
	a := newAzureWatching(t)
	a.track("Nightly")
	id := a.pushed("abc123")
	a.builds(
		azBuild{id: 11, pipeline: "CI", commit: "abc123", status: "completed", result: "partiallySucceeded", changed: "2026-01-05T09:05:00Z", finished: "2026-01-05T09:05:00Z"},
		azBuild{id: 12, pipeline: "Nightly", commit: "abc123", status: "completed", result: "failed", changed: "2026-01-05T09:04:00Z", finished: "2026-01-05T09:04:00Z"},
		azBuild{id: 13, pipeline: "CI", commit: "other", status: "completed", result: "failed", changed: "2026-01-05T09:03:00Z", finished: "2026-01-05T09:03:00Z"})
	a.timeline(11,
		azRecord{"Stage", "Build stage", "succeeded", 1},
		azRecord{"Job", "test", "succeeded", 1},
		azRecord{"Job", "lint", "succeededWithIssues", 1},
		azRecord{"Job", "docs", "skipped", 1},
		azRecord{"Job", "deploy", "abandoned", 1},
		azRecord{"Task", "npm test", "succeeded", 1})
	a.timeline(12, azRecord{"Job", "build", "failed", 1})
	a.startAzure()

	tk := a.ciResult(id, core.CIPassed)
	if len(tk.CIRuns) != 2 {
		t.Fatalf("runs = %+v, want the two builds of the commit, not the other commit's", tk.CIRuns)
	}
	ci, nightly := tk.CIRuns[0], tk.CIRuns[1]
	if ci.ID != 11 || ci.Workflow != "CI" || ci.Name != "20260105.11" || ci.Branch != "main" || ci.Commit != "abc123" || ci.Ticket != id ||
		ci.Status != store.CICompleted || ci.Conclusion != store.CIPartiallySucceeded || ci.URL != "https://dev.azure.com/acme/web/_build/results?buildId=11" ||
		ci.Started == nil || ci.Finished == nil || !ci.Finished.Equal(time.Date(2026, 1, 5, 9, 5, 0, 0, time.UTC)) || ci.Ignored {
		t.Fatalf("CI's run = %+v", ci)
	}
	want := []core.CIJobView{
		{Name: "test", Conclusion: store.CISuccess}, {Name: "lint", Conclusion: store.CIPartiallySucceeded},
		{Name: "docs", Conclusion: store.CISkipped}, {Name: "deploy", Conclusion: store.CICancelled}}
	if len(ci.Jobs) != len(want) {
		t.Fatalf("CI's jobs = %+v, want the Job records only", ci.Jobs)
	}
	for i, j := range ci.Jobs {
		if j.Name != want[i].Name || j.Conclusion != want[i].Conclusion || j.URL != fmt.Sprintf("https://dev.azure.com/acme/web/_build/results?buildId=11&view=logs&j=rec%d", i+1) {
			t.Fatalf("job %d = %+v, want %+v with its logs link", i, j, want[i])
		}
	}
	if !nightly.Ignored || nightly.Conclusion != store.CIFailure {
		t.Fatalf("Nightly's run = %+v, want its failure kept and marked ignored", nightly)
	}
}

func TestAnAzureStatusAndResultMapToCodeCentrals(t *testing.T) {
	for _, c := range []struct {
		status, result string
		wantResult     core.CIResult
		wantStatus     store.CIRunStatus
		wantConclusion store.CIConclusion
	}{
		{"notStarted", "", core.CIRunning, store.CIQueued, ""},
		{"postponed", "", core.CIRunning, store.CIQueued, ""},
		{"inProgress", "", core.CIRunning, store.CIInProgress, ""},
		{"cancelling", "", core.CIRunning, store.CIInProgress, ""},
		{"completed", "succeeded", core.CIPassed, store.CICompleted, store.CISuccess},
		{"completed", "failed", core.CIFailed, store.CICompleted, store.CIFailure},
		{"completed", "canceled", core.CICancelled, store.CICompleted, store.CICancelled},
	} {
		t.Run(c.status+"/"+c.result, func(t *testing.T) {
			a := newAzureWatching(t)
			id := a.pushed("abc123")
			finished := ""
			if c.status == "completed" {
				finished = "2026-01-05T09:05:00Z"
			}
			a.builds(azBuild{id: 11, pipeline: "CI", commit: "abc123", status: c.status, result: c.result, changed: "2026-01-05T09:05:00Z", finished: finished})
			a.startAzure()
			tk := a.ciResult(id, c.wantResult)
			if r := tk.CIRuns[0]; r.Status != c.wantStatus || r.Conclusion != c.wantConclusion {
				t.Fatalf("run = %+v, want %s %q", r, c.wantStatus, c.wantConclusion)
			}
		})
	}
}

func TestAnUnchangedAzureBuildCostsNoFurtherTimelineCall(t *testing.T) {
	a := newAzureWatching(t)
	id := a.pushed("abc123")
	a.builds(azBuild{id: 11, pipeline: "CI", commit: "abc123", status: "inProgress", changed: "2026-01-05T09:02:00Z"})
	a.timeline(11, azRecord{"Job", "test", "", 1})
	a.startAzure()
	a.ciResult(id, core.CIRunning)

	a.poll()
	a.poll()
	if n := a.requests("/timeline"); n != 1 {
		t.Fatalf("read the timeline %d times, want once for an unchanged build, shared by the run and its jobs", n)
	}

	// Changed, it is read again.
	a.builds(azBuild{id: 11, pipeline: "CI", commit: "abc123", status: "completed", result: "failed", changed: "2026-01-05T09:06:00Z", finished: "2026-01-05T09:06:00Z"})
	a.timeline(11, azRecord{"Job", "test", "failed", 1})
	a.poll()
	tk := a.ciResult(id, core.CIFailed)
	if tk.CIRuns[0].Jobs[0].Conclusion != store.CIFailure || a.requests("/timeline") != 2 {
		t.Fatalf("run = %+v after %d timeline reads, want its job failed after one more", tk.CIRuns[0], a.requests("/timeline"))
	}
}

func TestARetriedAzureBuildsAttemptIsItsHighestJobAttempt(t *testing.T) {
	a := newAzureWatching(t)
	id := a.pushed("abc123")
	a.builds(azBuild{id: 11, pipeline: "CI", commit: "abc123", status: "completed", result: "failed", changed: "2026-01-05T09:05:00Z", finished: "2026-01-05T09:05:00Z"})
	a.timeline(11, azRecord{"Job", "test", "failed", 1}, azRecord{"Job", "lint", "succeeded", 1})
	a.startAzure()
	if tk := a.ciResult(id, core.CIFailed); tk.CIRuns[0].Attempt != 1 {
		t.Fatalf("attempt = %d, want 1 before any retry", tk.CIRuns[0].Attempt)
	}

	a.builds(azBuild{id: 11, pipeline: "CI", commit: "abc123", status: "completed", result: "succeeded", changed: "2026-01-05T09:09:00Z", finished: "2026-01-05T09:09:00Z"})
	a.timeline(11, azRecord{"Job", "test", "succeeded", 2}, azRecord{"Job", "lint", "succeeded", 1})
	a.poll()
	tk := a.ciResult(id, core.CIPassed)
	var latest *core.CIRunView
	for i, r := range tk.CIRuns {
		if r.Attempt == 2 {
			latest = &tk.CIRuns[i]
		}
	}
	if latest == nil || len(latest.Jobs) != 2 || latest.Jobs[0].Conclusion != store.CISuccess {
		t.Fatalf("runs = %+v, want an attempt 2 with its jobs at their latest attempt", tk.CIRuns)
	}
}

// rulePollAzure waits for a poll to list the rules' branches, then for
// the loop to wait again.
func (a *azureWatching) rulePollAzure() {
	a.t.Helper()
	count := func() int {
		n := 0
		for _, r := range a.azdo.Requests() {
			if r.Path == buildsPath && len(r.Query["statusFilter"]) > 0 {
				n++
			}
		}
		return n
	}
	n := count()
	if n > 0 {
		a.srv.Clock.BlockUntil(1)
		a.srv.Clock.Advance(ciwatcher.Interval)
	}
	a.waitFor("a poll listing the rules' branches", func() bool { return count() > n })
	a.srv.Clock.BlockUntil(1)
}

func TestAFailingAzureBuildARuleMatchesOpensAFixTicketNamingItsFailedJobs(t *testing.T) {
	a := newAzureWatching(t)
	spec := a.rule("My Pipeline", "main", store.Ready)
	finished := at(5).Format(time.RFC3339)
	a.builds(
		azBuild{id: 21, pipeline: "My Pipeline", commit: "fff111", status: "completed", result: "failed", changed: finished, finished: finished},
		azBuild{id: 22, pipeline: "My Pipeline", branch: "refs/pull/7/merge", commit: "fff111", status: "completed", result: "failed", changed: finished, finished: finished},
		azBuild{id: 23, pipeline: "Other", commit: "fff111", status: "completed", result: "failed", changed: finished, finished: finished})
	a.timeline(21, azRecord{"Job", "test", "failed", 1}, azRecord{"Job", "lint", "succeeded", 1}, azRecord{"Stage", "Build", "failed", 1})
	a.startAzure()
	a.rulePollAzure()

	q := a.azdo.Requests()
	var listing map[string][]string
	for _, r := range q {
		if len(r.Query["statusFilter"]) > 0 {
			listing = r.Query
		}
	}
	if listing["branchName"][0] != "refs/heads/main" {
		t.Fatalf("listing query = %v, want the branch with its refs/heads/ prefix", listing)
	}
	tks := a.fixTickets(spec)
	if len(tks) != 1 {
		t.Fatalf("Tickets on %s = %d, want one fix Ticket, for the build on main of the rule's pipeline only", spec, len(tks))
	}
	tk := tks[0]
	if !strings.Contains(tk.Title, "My Pipeline") || !strings.Contains(tk.Title, "main") {
		t.Fatalf("title %q, want the pipeline and branch", tk.Title)
	}
	for _, want := range []string{"https://dev.azure.com/acme/web/_build/results?buildId=21", "fff111", "[test](https://dev.azure.com/acme/web/_build/results?buildId=21&view=logs&j=rec0)"} {
		if !strings.Contains(tk.Description, want) {
			t.Fatalf("description lacks %q:\n%s", want, tk.Description)
		}
	}
	if strings.Contains(tk.Description, "lint") || strings.Contains(tk.Description, "Build](") {
		t.Fatalf("description names a passed job or a stage:\n%s", tk.Description)
	}
	if got := a.lastRun(spec); got == nil || !got.Equal(at(5)) {
		t.Fatalf("rule's last run = %v, want build 21 tracked against it", got)
	}
}

func TestAnAnyRuleMatchesAnAzurePipelineWhateverItsName(t *testing.T) {
	a := newAzureWatching(t)
	spec := a.rule("any", "main", store.Draft)
	finished := at(5).Format(time.RFC3339)
	a.builds(azBuild{id: 21, pipeline: "Deploy web", commit: "fff111", status: "completed", result: "failed", changed: finished, finished: finished})
	a.timeline(21, azRecord{"Job", "ship", "failed", 1})
	a.startAzure()
	a.rulePollAzure()
	if tks := a.fixTickets(spec); len(tks) != 1 {
		t.Fatalf("Tickets on %s = %d, want one fix Ticket", spec, len(tks))
	}
}

// pullRequestPushed makes the pushed commit's Ticket have pull request 7.
func (a *azureWatching) pullRequestPushed(commit string) string {
	a.t.Helper()
	id := a.pushed(commit)
	ctx, human := context.Background(), core.WithActor(context.Background(), core.Human)
	c := a.srv.Core
	must(a.t, func() error {
		_, err := c.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: "https://dev.azure.com/acme/web/_git/web", Base: "main"})
		return err
	})
	must(a.t, func() error {
		_, err := c.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest})
		return err
	})
	must(a.t, func() error {
		_, err := c.ReportPullRequest(ctx, "acme", core.ReportPullRequestInput{Branch: id, Number: 7, Link: "https://dev.azure.com/acme/web/_git/web/pullrequest/7", State: store.PROpen})
		return err
	})
	return id
}

func TestAnAzureBuildOnThePullRequestsMergeRefCountsTowardsThePushedCommit(t *testing.T) {
	a := newAzureWatching(t)
	id := a.pullRequestPushed("abc123")
	a.builds(
		azBuild{id: 11, pipeline: "CI", branch: "refs/pull/7/merge", commit: "mergesha", status: "completed", result: "failed", changed: "2026-01-05T09:05:00Z", finished: "2026-01-05T09:05:00Z"},
		azBuild{id: 12, pipeline: "CI", branch: "refs/pull/8/merge", commit: "othermerge", status: "completed", result: "failed", changed: "2026-01-05T09:04:00Z", finished: "2026-01-05T09:04:00Z"})
	a.timeline(11, azRecord{"Job", "test", "failed", 1})
	a.startAzure()

	tk := a.ciResult(id, core.CIFailed)
	if len(tk.CIRuns) != 1 || tk.CIRuns[0].ID != 11 || tk.CIRuns[0].Commit != "abc123" || tk.CIRuns[0].Ticket != id {
		t.Fatalf("runs = %+v, want only the pull request 7 merge build, counted on the pushed commit", tk.CIRuns)
	}
}

func TestAnAzureRerunActsOnABuildOnThePullRequestsMergeRef(t *testing.T) {
	a := newAzureWatching(t)
	id := a.pullRequestPushed("abc123")
	a.builds(azBuild{id: 11, pipeline: "CI", branch: "refs/pull/7/merge", commit: "mergesha", status: "completed", result: "failed", changed: "2026-01-05T09:05:00Z", finished: "2026-01-05T09:05:00Z"})
	a.timeline(11, azRecord{"Job", "test", "failed", 1})
	a.startAzure()
	a.ciResult(id, core.CIFailed)
	human := core.WithActor(context.Background(), core.Human)
	must(t, func() error {
		_, err := a.srv.Core.RerunCI(human, "acme", core.TicketInput{Ticket: core.Ref(id)})
		return err
	})
	a.poll()
	a.waitFor("the build retried", func() bool { return len(a.retries()) == 1 })
}
