package pullrequestwatcher_test

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/internal/fakeazdo"
	"github.com/tomoram/code-central/internal/fakegh"
	"github.com/tomoram/code-central/pullrequestwatcher"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/watchers/watchertest"
)

// Pull requests per Repository (Spec code-central-848, ADR 0050): one
// Pull Request Watcher for the Project opens a pull request per Changed
// Repository on that Repository's own Code Host, keeps each body's list
// of siblings up to date, closes them on send back, and names a failing
// Repository while it carries on with the others.

const (
	apiRepoPath = fakeazdo.ProjectPath + "/_apis/git/repositories/api"
	apiPRsPath  = apiRepoPath + "/pullrequests"
	apiLink     = "/_git/api/pullrequest/4"
)

// severalWatching is a real Code Central with Pull Request on for acme,
// whose Home Repository is on GitHub (acme/web) and whose api Repository
// is on Azure Repos, with acme-2 Accepted with work in both, each pushed
// as acme-2; and the Pull Request Watcher running against the fake gh and
// the fake Azure DevOps.
type severalWatching struct {
	t     *testing.T
	h     *watchertest.Harness
	azure *fakeazdo.Server
	srv   *codecentraltest.Server
}

func newSeveralWatching(t *testing.T, azure ...fakeazdo.Answer) *severalWatching {
	t.Helper()
	srv := codecentraltest.NewServer(t)
	c := srv.Core
	must(t, func() error { _, err := c.CreateProject(human, core.CreateProjectInput{Key: "acme"}); return err })
	must(t, func() error {
		_, err := c.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: "git@github.com:acme/web.git", Base: "main"})
		return err
	})
	must(t, func() error {
		_, err := c.AddRepository(human, "acme", core.AddRepositoryInput{Name: "api", CloneURL: "https://dev.azure.com/acme/shop/_git/api", Base: "develop"})
		return err
	})
	must(t, func() error {
		_, err := c.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest})
		return err
	})
	must(t, func() error { _, err := c.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Login"}); return err })
	must(t, func() error {
		_, err := c.AddLabels(ctx, "acme", core.LabelsInput{Target: "acme-1", Labels: []string{core.LongLivedLabel}})
		return err
	})
	must(t, func() error {
		_, err := c.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Login form", State: store.Ready})
		return err
	})
	res, err := c.Claim(ctx, "acme", core.ClaimInput{Ticket: "2"})
	if err != nil {
		t.Fatal(err)
	}
	must(t, func() error {
		_, err := c.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: res.ClaimID, Summary: "Built the login form.", Branch: "acme-2", Commit: "abc123",
			Repositories: []core.RepositoryWork{{Name: "api", Branch: "acme-2", Commit: "def456"}}})
		return err
	})
	must(t, func() error { _, err := c.Accept(human, "acme", core.TicketInput{Ticket: "2"}); return err })
	for _, in := range []core.ReportPushedCommitInput{{Ticket: "acme-2", Commit: "abc123", Branch: "acme-2"}, {Ticket: "acme-2", Commit: "def456", Branch: "acme-2", Repository: "api"}} {
		must(t, func() error { _, err := srv.Client.ReportPushedCommit(ctx, "acme", in); return err })
	}
	h := watchertest.New(t)
	h.Answer(fakegh.Answer{Args: []string{"api", repoPath}, Stdout: write})
	return &severalWatching{t: t, h: h, azure: fakeazdo.New(t, azure...), srv: srv}
}

// start runs the loop with environ added to gh's, such as the PAT.
func (w *severalWatching) start(environ ...string) {
	w.t.Helper()
	r, pw := io.Pipe()
	cctx, cancel := context.WithCancel(context.Background())
	exited := make(chan struct{})
	w.t.Cleanup(func() {
		cancel()
		pw.Close()
		<-exited
	})
	go func() {
		defer close(exited)
		_ = pullrequestwatcher.Watch(cctx, pullrequestwatcher.Config{
			Client: client.New(w.srv.URL), Project: "acme", Clock: w.srv.Clock, Stdin: r, Log: io.Discard,
			Env: gh.Env{Environ: append(w.h.Environ(w.h.Bin), environ...), InstallPlaces: []string{}}, AzureProjectURL: w.azure.ProjectURL(),
		})
	}()
}

// pullRequestIn waits for acme-2's pull request in repository to be in
// state.
func (w *severalWatching) pullRequestIn(repository string, state store.PullRequestState) core.PullRequestView {
	w.t.Helper()
	var got *core.PullRequestView
	codecentraltest.Eventually(w.t, func() bool {
		tk, err := w.srv.Core.GetTicket(ctx, "acme", core.GetTicketInput{Ticket: "2"})
		if err != nil {
			return false
		}
		for _, r := range tk.Repositories {
			if r.Name == repository {
				got = r.PullRequest
			}
		}
		return got != nil && got.State == state
	}, "acme-2's pull request in "+repository+" is %+v, want "+string(state), &got)
	return *got
}

// ghCalls is the gh calls whose args start with prefix.
func (w *severalWatching) ghCalls(prefix ...string) [][]string {
	var out [][]string
	for _, c := range w.h.Calls() {
		if len(c.Args) >= len(prefix) && slices.Equal(c.Args[:len(prefix)], prefix) {
			out = append(out, c.Args)
		}
	}
	return out
}

// azureCalls is the requests to the fake Azure DevOps with method and
// path.
func (w *severalWatching) azureCalls(method, path string) []fakeazdo.Request {
	var out []fakeazdo.Request
	for _, r := range w.azure.Requests() {
		if r.Method == method && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

func TestTwoRepositoriesOnMixedCodeHostsGetTwoPullRequestsThatReferenceEachOther(t *testing.T) {
	w := newSeveralWatching(t,
		fakeazdo.JSON("GET", apiPRsPath, 200, map[string]any{"count": 0, "value": []any{}}),
		fakeazdo.JSON("POST", apiPRsPath, 201, apr(4, "active", "queued", nil)))
	w.h.Answer(fakegh.Answer{Args: []string{"api", listPath}, Stdout: "[]"})
	w.h.Answer(fakegh.Answer{Args: []string{"api", repoPath + "/pulls", "--method", "POST"}, Stdout: pr(7, "open", "")})
	w.start("ADO_PAT=" + secretPAT)

	home := w.pullRequestIn("acme", store.PROpen)
	api := w.pullRequestIn("api", store.PROpen)
	if home.Number != 7 || api.Number != 4 || !strings.HasSuffix(api.Link, apiLink) {
		t.Fatalf("pull requests acme %+v, api %+v", home, api)
	}
	// Each opens into its own Repository's Base.
	opened := w.ghCalls("api", repoPath+"/pulls", "--method", "POST")
	if len(opened) != 1 || !slices.Contains(opened[0], "base=main") {
		t.Fatalf("GitHub opens %q, want one into main", opened)
	}
	posted := w.azureCalls("POST", apiPRsPath)
	if len(posted) != 1 || !strings.Contains(posted[0].Body, `"targetRefName":"refs/heads/develop"`) || !strings.Contains(posted[0].Body, `- acme: not opened yet\n- api: this pull request`) {
		t.Fatalf("Azure Repos opens %+v, want one into develop listing its siblings", posted)
	}

	// Each host lists its pull request with the body it opened with, from
	// before its sibling opened: the next poll edits each to link the
	// other's.
	w.h.Answer(fakegh.Answer{Args: []string{"api", listPath}, Stdout: "[" + pr(7, "open", `"body":"old"`) + "]"})
	w.azure.Script(fakeazdo.JSON("GET", apiPRsPath, 200, map[string]any{"count": 1, "value": []any{apr(4, "active", "succeeded", map[string]any{"description": "old"})}}))
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(pullrequestwatcher.Interval)
	var edits [][]string
	codecentraltest.Eventually(t, func() bool {
		edits = w.ghCalls("api", repoPath+"/pulls/7", "--method", "PATCH")
		return len(edits) > 0 && len(w.azureCalls("PATCH", apiPRsPath+"/4")) > 0
	}, "GitHub edits %q, want each body edited", &edits)
	body := strings.Join(edits[0], "\x00")
	if !strings.Contains(body, "acme: this pull request") || !strings.Contains(body, "api: "+w.azure.URL+fakeazdo.ProjectPath+apiLink) {
		t.Errorf("GitHub edit %q, want the siblings listed with api's link", edits[0])
	}
	if got := w.azureCalls("PATCH", apiPRsPath+"/4")[0].Body; !strings.Contains(got, `- acme: https://github.com/acme/web/pull/7\n- api: this pull request`) {
		t.Errorf("Azure Repos edit %s, want the siblings listed with acme's link", got)
	}
}

func TestEachPullRequestMergesOnItsOwnHostAndIsRecordedInItsRepository(t *testing.T) {
	w := newSeveralWatching(t, fakeazdo.JSON("GET", apiPRsPath, 200, map[string]any{"count": 1, "value": []any{
		apr(4, "completed", "succeeded", map[string]any{"closedDate": "2099-01-02T03:04:05Z", "lastMergeCommit": map[string]string{"commitId": "m-api"}})}}))
	w.h.Answer(fakegh.Answer{Args: []string{"api", listPath}, Stdout: "[" + pr(7, "open", "") + "]"})
	w.start("ADO_PAT=" + secretPAT)

	// api merges first, while the Home Repository's stays open.
	if got := w.pullRequestIn("api", store.PRMerged); got.MergeCommit != "m-api" {
		t.Fatalf("api merged at %q, want m-api", got.MergeCommit)
	}
	w.pullRequestIn("acme", store.PROpen)
	w.h.Answer(fakegh.Answer{Args: []string{"api", listPath}, Stdout: "[" + pr(7, "closed", `"merged_at":"2099-01-02T03:04:05Z","closed_at":"2099-01-02T03:04:05Z","merge_commit_sha":"m-web"`) + "]"})
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(pullrequestwatcher.Interval)
	if got := w.pullRequestIn("acme", store.PRMerged); got.MergeCommit != "m-web" {
		t.Fatalf("acme merged at %q, want m-web", got.MergeCommit)
	}
}

func TestSendingBackClosesEachOpenPullRequestOnItsHost(t *testing.T) {
	w := newSeveralWatching(t,
		fakeazdo.JSON("GET", apiPRsPath, 200, map[string]any{"count": 1, "value": []any{apr(4, "active", "succeeded", nil)}}),
		fakeazdo.JSON("PATCH", apiPRsPath+"/4", 200, apr(4, "abandoned", "succeeded", nil)))
	w.h.Answer(fakegh.Answer{Args: []string{"api", listPath}, Stdout: "[" + pr(7, "open", "") + "]"})
	w.h.Answer(fakegh.Answer{Args: []string{"api", repoPath + "/pulls/7", "--method", "PATCH", "-f", "state=closed"}, Stdout: pr(7, "closed", "")})
	w.start("ADO_PAT=" + secretPAT)
	w.pullRequestIn("acme", store.PROpen)
	w.pullRequestIn("api", store.PROpen)

	must(t, func() error {
		_, err := w.srv.Core.SendBack(human, "acme", core.SendBackInput{Ticket: "2", Note: "Redo it."})
		return err
	})
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(pullrequestwatcher.Interval)
	w.pullRequestIn("acme", store.PRClosed)
	w.pullRequestIn("api", store.PRClosed)
	abandoned := slices.ContainsFunc(w.azureCalls("PATCH", apiPRsPath+"/4"), func(r fakeazdo.Request) bool { return strings.Contains(r.Body, `"status":"abandoned"`) })
	if !abandoned || len(w.ghCalls("api", repoPath+"/pulls/7", "--method", "PATCH", "-f", "state=closed")) != 1 {
		t.Errorf("want api's pull request abandoned and acme's closed")
	}
	watch, err := w.srv.Core.GetPullRequestWatch(ctx, "acme")
	if err != nil || len(watch.Close) != 0 {
		t.Errorf("watch close %+v (%v), want nothing left to close", watch.Close, err)
	}
}

func TestAFailingRepositoryIsNamedAndTheOthersAreStillWatched(t *testing.T) {
	w := newSeveralWatching(t)
	w.h.Answer(fakegh.Answer{Args: []string{"api", listPath}, Stdout: "[]"})
	w.h.Answer(fakegh.Answer{Args: []string{"api", repoPath + "/pulls", "--method", "POST"}, Stdout: pr(7, "open", "")})
	w.start() // no PAT: api can't be watched

	w.pullRequestIn("acme", store.PROpen)
	var got core.WatcherView
	codecentraltest.Eventually(t, func() bool {
		p, err := w.srv.Client.GetProject(ctx, "acme")
		if err != nil {
			t.Fatal(err)
		}
		got = p.Landing.Watcher
		return got.State == core.WatcherError
	}, "the Pull Request Watcher's status %+v, want error", &got)
	if len(got.Repositories) != 1 || got.Repositories[0].Repository != "api" || got.Repositories[0].Problem != core.ProblemPATMissing || got.Repositories[0].Fix == "" {
		t.Errorf("status %+v, want api named with its missing PAT and fix", got)
	}
	if !strings.Contains(got.Message, "api: ") {
		t.Errorf("message %q, want api named", got.Message)
	}
}

func TestTurningPullRequestOnChecksEachRepositoryNamingTheOneThatFails(t *testing.T) {
	h := watchertest.New(t)
	h.Answer(fakegh.Answer{Args: []string{"api", repoPath}, Stdout: write})
	h.Answer(fakegh.Answer{Args: []string{"api", "repos/acme/api"}, Stdout: readOnly})
	srv := checking(t, h, h.Bin)
	must(t, func() error {
		_, err := srv.Core.AddRepository(human, "acme", core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})
		return err
	})

	err := turnOn(srv)
	var e *core.Error
	if !asError(err, &e) || e.Field != "method" || !strings.HasPrefix(e.Message, "api: ") || !strings.Contains(e.Message, "acme/api") || !strings.HasPrefix(e.Fix, "api: ") {
		t.Fatalf("turning on = %v, want refused naming api with its reason and fix", err)
	}
	if strings.Contains(e.Message, "acme: ") {
		t.Errorf("refusal %q names the Home Repository, which passed", e.Message)
	}
	if got := method(t, srv); got == store.LandingPullRequest {
		t.Fatal("Pull Request turned on")
	}

	// Once api is writable it turns on, every Repository checked.
	h.Answer(fakegh.Answer{Args: []string{"api", "repos/acme/api"}, Stdout: write})
	if err := turnOn(srv); err != nil {
		t.Fatalf("turning on: %v", err)
	}
	var read []string
	for _, c := range h.Calls() {
		if len(c.Args) > 1 && c.Args[0] == "api" {
			read = append(read, c.Args[1])
		}
	}
	if !slices.Contains(read, repoPath) || !slices.Contains(read, "repos/acme/api") {
		t.Errorf("gh read %q, want both repositories", read)
	}
}
