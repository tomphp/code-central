package pullrequestwatcher_test

import (
	"context"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/internal/fakeazdo"
	"github.com/tomoram/code-central/pullrequestwatcher"
	"github.com/tomoram/code-central/store"
)

const (
	azureRemote = "https://dev.azure.com/acme/web/_git/web"
	secretPAT   = "s3cret-pat-value"
	reposPath   = fakeazdo.ProjectPath + "/_apis/git/repositories/web"
	prsPath     = reposPath + "/pullrequests"
)

func azureCheck(srv *fakeazdo.Server, environ ...string) core.WatcherCheckResult {
	in := core.PullRequestCheckInput{CodeHost: core.CodeHostAzureRepos, Repository: "acme/web/web", PATEnv: "ADO_PAT", ProjectURL: srv.ProjectURL()}
	return pullrequestwatcher.Check(context.Background(), in, gh.Env{Environ: environ})
}

func TestAnAzureCheckWithAWorkingPATPassesAndCreatesNoPullRequest(t *testing.T) {
	srv := fakeazdo.New(t, fakeazdo.JSON("POST", prsPath, 400, map[string]string{"message": "TF401398: source branch not found"}))
	if r := azureCheck(srv, "ADO_PAT="+secretPAT); r != (core.WatcherCheckResult{}) {
		t.Fatalf("check = %+v, want no problem", r)
	}
	reqs := srv.Requests()
	if len(reqs) != 2 || reqs[0].Method != "GET" || !strings.HasSuffix(reqs[0].Path, "/repositories/web") || reqs[1].Method != "POST" {
		t.Fatalf("requests = %+v, want the repository read, then one refused pull request probe", reqs)
	}
	if !strings.Contains(reqs[1].Body, "no-such-branch") {
		t.Fatalf("probe body = %s, want a pull request from a branch that doesn't exist", reqs[1].Body)
	}
	if want := "Basic OnMzY3JldC1wYXQtdmFsdWU="; reqs[0].Authorization != want {
		t.Fatalf("Authorization = %q, want %q", reqs[0].Authorization, want)
	}
}

func TestAnUnsetOrEmptyAzurePATVariableIsPATMissing(t *testing.T) {
	for name, environ := range map[string][]string{"unset": nil, "empty": {"ADO_PAT="}} {
		t.Run(name, func(t *testing.T) {
			srv := fakeazdo.New(t)
			r := azureCheck(srv, environ...)
			if r.Problem != core.ProblemPATMissing || !strings.Contains(r.Reason, "ADO_PAT") || !strings.Contains(r.Fix, "restart") || !strings.Contains(r.Fix, "Code: Read & write") {
				t.Fatalf("check = %+v, want pat_missing naming ADO_PAT, the scope and a restart", r)
			}
			want := map[string]string{"darwin": "launchctl setenv ADO_PAT", "linux": "systemctl --user set-environment", "windows": "user environment variable"}[runtime.GOOS]
			if !strings.Contains(r.Fix, want) {
				t.Fatalf("fix = %q, want it to say %q", r.Fix, want)
			}
			if n := len(srv.Requests()); n != 0 {
				t.Fatalf("%d requests, want none without a PAT", n)
			}
		})
	}
}

func TestEachAzureRefusalIsItsOwnProblem(t *testing.T) {
	for name, tc := range map[string]struct {
		answers []fakeazdo.Answer
		problem core.WatcherProblem
		fix     string
	}{
		"rejected":          {[]fakeazdo.Answer{{Path: "/", Status: 401}}, core.ProblemPATRejected, "Code: Read & write"},
		"repo not found":    {[]fakeazdo.Answer{{Path: reposPath, Method: "GET", Status: 404}}, core.ProblemRepositoryUnreadable, "Push remote"},
		"no contribute":     {[]fakeazdo.Answer{{Path: prsPath, Method: "POST", Status: 403}}, core.ProblemPATScope, "Code: Read & write"},
		"read scope absent": {[]fakeazdo.Answer{{Path: reposPath, Method: "GET", Status: 403}}, core.ProblemPATScope, "Code: Read & write"},
	} {
		t.Run(name, func(t *testing.T) {
			r := azureCheck(fakeazdo.New(t, tc.answers...), "ADO_PAT="+secretPAT)
			if r.Problem != tc.problem || !strings.Contains(r.Fix, tc.fix) {
				t.Fatalf("check = %+v, want %s with a fix naming %q", r, tc.problem, tc.fix)
			}
			if strings.Contains(r.Reason+r.Fix, secretPAT) {
				t.Fatal("the PAT leaked into the check result")
			}
		})
	}
}

func TestAnUnreachableAzureHostIsRepositoryUnreadable(t *testing.T) {
	srv := fakeazdo.New(t)
	in := core.PullRequestCheckInput{CodeHost: core.CodeHostAzureRepos, Repository: "acme/web/web", PATEnv: "ADO_PAT", ProjectURL: srv.ProjectURL()}
	srv.Close()
	r := pullrequestwatcher.Check(context.Background(), in, gh.Env{Environ: []string{"ADO_PAT=" + secretPAT}})
	if r.Problem != core.ProblemRepositoryUnreadable {
		t.Fatalf("check = %+v, want repository_unreadable", r)
	}
}

func TestARateLimitedAzureCheckFailsWithoutAProblemOfItsOwn(t *testing.T) {
	srv := fakeazdo.New(t, fakeazdo.Answer{Path: "/", Status: 429}.WithHeader("Retry-After", "60"))
	r := azureCheck(srv, "ADO_PAT="+secretPAT)
	if r.Problem != core.ProblemCheckFailed || !strings.Contains(r.Reason, "throttling") {
		t.Fatalf("check = %+v, want check_failed about the rate limit", r)
	}
}

func TestTurningPullRequestOnForAnAzureReposRemoteIsRefusedWithReasonAndFixWhenTheCheckFails(t *testing.T) {
	azure := fakeazdo.New(t)
	check := func(ctx context.Context, in core.PullRequestCheckInput) core.WatcherCheckResult {
		if in.CodeHost != core.CodeHostAzureRepos || in.Repository != "acme/web/web" || in.PATEnv != "MY_PAT" {
			t.Errorf("check input = %+v, want Azure Repos acme/web/web with MY_PAT", in)
		}
		in.ProjectURL = azure.ProjectURL()
		return pullrequestwatcher.Check(ctx, in, gh.Env{Environ: nil})
	}
	srv := codecentraltest.NewServer(t, codecentraltest.WithPullRequestCheck(check))
	must(t, func() error {
		_, err := srv.Core.CreateProject(human, core.CreateProjectInput{Key: "acme"})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: azureRemote, Base: "main"})
		return err
	})
	_, err := srv.Core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest, PATEnv: "MY_PAT"})
	var e *core.Error
	if !asError(err, &e) || e.Field != "pat_env" || !strings.Contains(e.Message, "MY_PAT") || !strings.Contains(e.Fix, "restart") {
		t.Fatalf("err = %v, want a refusal on the PAT variable with its reason and fix", err)
	}
	if got := method(t, srv); got == store.LandingPullRequest {
		t.Fatal("Pull Request was turned on despite the failed check")
	}
}

// azureWatching is newWatching with an Azure Repos Push remote and the loop
// running against the fake Azure DevOps.
type azureWatching struct {
	*watching
	azure *fakeazdo.Server
}

func newAzureWatching(t *testing.T, answers ...fakeazdo.Answer) *azureWatching {
	t.Helper()
	return &azureWatching{watching: newWatchingOn(t, azureRemote), azure: fakeazdo.New(t, answers...)}
}

func (w *azureWatching) start(environ ...string) {
	w.t.Helper()
	r, pw := io.Pipe()
	w.stdin, w.done = pw, make(chan error, 1)
	cctx, cancel := context.WithCancel(context.Background())
	exited := make(chan struct{})
	// Wait for the loop to stop, so that no gh call is still writing into the
	// TempDir when its cleanup removes it.
	w.t.Cleanup(func() {
		cancel()
		pw.Close()
		<-exited
	})
	go func() {
		defer close(exited)
		w.done <- pullrequestwatcher.Watch(cctx, pullrequestwatcher.Config{
			Client: client.New(w.srv.URL), Project: "acme", Clock: w.srv.Clock, Stdin: r, Log: io.Discard,
			Env: gh.Env{Environ: environ}, AzureProjectURL: w.azure.ProjectURL(),
		})
	}()
}

func (w *azureWatching) requests(method, path string) []fakeazdo.Request {
	var out []fakeazdo.Request
	for _, r := range w.azure.Requests() {
		if r.Method == method && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

func apr(id int, status, mergeStatus string, extra map[string]any) map[string]any {
	m := map[string]any{"pullRequestId": id, "status": status, "mergeStatus": mergeStatus}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func list(prs ...map[string]any) fakeazdo.Answer {
	return fakeazdo.JSON("GET", prsPath, 200, map[string]any{"count": len(prs), "value": prs})
}

func TestItOpensAReadyAzurePullRequestIntoTheBaseWithTheTicketsTitleAndSummaryAndReportsIt(t *testing.T) {
	w := newAzureWatching(t, list(), fakeazdo.JSON("POST", prsPath, 201, apr(7, "active", "queued", nil)))
	w.start("ADO_PAT=" + secretPAT)
	got := w.recorded(store.PROpen)
	if got.Number != 7 || got.Link != w.azure.ProjectURL()+"/_git/web/pullrequest/7" {
		t.Fatalf("recorded %+v, want pull request 7 and its link", got)
	}
	w.status(core.WatcherWatching)
	opened := w.requests("POST", prsPath)
	if len(opened) != 1 {
		t.Fatalf("%d pull requests opened, want one", len(opened))
	}
	for _, want := range []string{`"sourceRefName":"refs/heads/acme-2"`, `"targetRefName":"refs/heads/main"`, `"title":"Login form"`, `"isDraft":false`, "Built the login form.", "acme-2"} {
		if !strings.Contains(opened[0].Body, want) {
			t.Errorf("opened with %s, want it to contain %s", opened[0].Body, want)
		}
	}
	q := w.requests("GET", prsPath)[0].Query
	if q["searchCriteria.sourceRefName"][0] != "refs/heads/acme-2" || q["searchCriteria.targetRefName"][0] != "refs/heads/main" || q["searchCriteria.status"][0] != "all" {
		t.Fatalf("list query = %v, want the branch into the base, every status", q)
	}
}

func TestItAdoptsAnActiveAzurePullRequestInsteadOfOpeningOne(t *testing.T) {
	w := newAzureWatching(t, list(apr(4, "active", "succeeded", nil)))
	w.start("ADO_PAT=" + secretPAT)
	if got := w.recorded(store.PROpen); got.Number != 4 {
		t.Fatalf("recorded %+v, want the adopted pull request 4", got)
	}
	if n := len(w.requests("POST", prsPath)); n != 0 {
		t.Fatalf("%d pull requests opened, want none", n)
	}
}

func TestItMapsAzureStatusesAndReportsTheMergeCommit(t *testing.T) {
	closed := "2026-11-11T12:00:00Z"
	for name, tc := range map[string]struct {
		pr    map[string]any
		state store.PullRequestState
		merge string
	}{
		"active":      {apr(5, "active", "succeeded", nil), store.PROpen, ""},
		"conflicting": {apr(5, "active", "conflicts", nil), store.PRConflicting, ""},
		"abandoned":   {apr(5, "abandoned", "succeeded", map[string]any{"closedDate": closed}), store.PRClosed, ""},
		"completed": {apr(5, "completed", "succeeded", map[string]any{"closedDate": closed,
			"lastMergeCommit": map[string]string{"commitId": "deadbeef"}}), store.PRMerged, "deadbeef"},
	} {
		t.Run(name, func(t *testing.T) {
			w := newAzureWatching(t, list(tc.pr))
			w.start("ADO_PAT=" + secretPAT)
			got := w.recorded(tc.state)
			if got.MergeCommit != tc.merge {
				t.Fatalf("recorded %+v, want merge commit %q", got, tc.merge)
			}
		})
	}
}

func TestAnAbandonedAzurePullRequestIsPolledSoAReopenedOneIsSeen(t *testing.T) {
	closed := map[string]any{"closedDate": "2026-11-11T12:00:00Z"}
	w := newAzureWatching(t, list(apr(5, "abandoned", "succeeded", closed)))
	w.start("ADO_PAT=" + secretPAT)
	w.recorded(store.PRClosed)
	w.azure.Script(list(apr(5, "active", "succeeded", nil)))
	w.nextPoll(pullrequestwatcher.Interval)
	if got := w.recorded(store.PROpen); got.Number != 5 {
		t.Fatalf("recorded %+v, want pull request 5 open again", got)
	}
	if n := len(w.requests("POST", prsPath)); n != 0 {
		t.Fatalf("%d pull requests opened, want none", n)
	}
}

func TestANewAzurePullRequestOpensAfterAnAbandonedOneWhenTheLatestPushIsNewer(t *testing.T) {
	// The abandoned one closed long before the push.
	old := map[string]any{"closedDate": "2020-01-02T00:00:00Z", "creationDate": "2020-01-01T00:00:00Z"}
	w := newAzureWatching(t, list(apr(5, "abandoned", "succeeded", old)), fakeazdo.JSON("POST", prsPath, 201, apr(6, "active", "queued", nil)))
	w.start("ADO_PAT=" + secretPAT)
	if got := w.recorded(store.PROpen); got.Number != 6 {
		t.Fatalf("recorded %+v, want the new pull request 6", got)
	}
}

func TestAnAzureRateLimitStopsCallsUntilTheResetAndIsNeverAnError(t *testing.T) {
	w := newAzureWatching(t)
	reset := w.srv.Clock.Now().Add(15 * time.Minute)
	w.azure.Script(fakeazdo.Answer{Path: "/", Status: http.StatusTooManyRequests}.WithHeader("X-RateLimit-Reset", itoa(reset.Unix())))
	w.start("ADO_PAT=" + secretPAT)
	var got core.WatcherView
	codecentraltest.Eventually(t, func() bool {
		p, err := w.srv.Client.GetProject(ctx, "acme")
		if err != nil {
			t.Fatal(err)
		}
		got = p.Landing.Watcher
		return got.RateLimitedUntil.Equal(reset)
	}, "status %+v, want Watching until %s", &got, reset)
	if got.State != core.WatcherWatching {
		t.Fatalf("status %+v, want Watching", got)
	}
	calls := len(w.azure.Requests())
	w.nextPoll(15*time.Minute - time.Second)
	if n := len(w.azure.Requests()); n != calls {
		t.Fatalf("%d requests before the reset, want none after %d", n, calls)
	}
	if n := w.errorCount(); n != 0 {
		t.Fatalf("went into an error %d times, want none", n)
	}
}

func TestAMissingAzurePATIsAnErrorWithItsFixThatClearsWhenFixed(t *testing.T) {
	w := newAzureWatching(t, list(apr(4, "active", "succeeded", nil)))
	w.start()
	got := w.status(core.WatcherError)
	if got.Problem != core.ProblemPATMissing || !strings.Contains(got.Message, "ADO_PAT") || got.Fix == "" {
		t.Fatalf("status %+v, want pat_missing naming ADO_PAT with a fix", got)
	}
}

func asError(err error, target **core.Error) bool {
	e, ok := err.(*core.Error)
	if ok {
		*target = e
	}
	return ok
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
