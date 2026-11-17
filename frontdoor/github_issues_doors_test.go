package frontdoor_test

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
)

// GitHub Issues settings (code-central-284): set_github_issues is the
// human's, so HTTP only. Turning it on first creates the Project's Issues
// Spec, acme-4 on the cross-door server.
func init() {
	cases["set_github_issues"] = opCase{
		ok: call{http: setGitHubIssues(`{"on":true,"repositories":[" acme/web "],"label":" triage ","new_issues_need_me":true}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "github_issues")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			field("key", "acme")(t, out)
			githubIssues(t, out, issuesSettings{on: true, repositories: "acme/web", label: "triage", needMe: true, spec: "acme-4"})
		},
		bad:  call{http: setGitHubIssues(`{"on":true}`)},
		code: core.Invalid,
	}
}

func setGitHubIssues(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/set_github_issues", body}
}

// issuesSettings is a Project's github_issues object.
type issuesSettings struct {
	on           bool
	repositories string
	label        string
	needMe       bool
	spec         string
}

// githubIssues checks a Project's github_issues object is want.
func githubIssues(t *testing.T, project map[string]any, want issuesSettings) {
	t.Helper()
	g, _ := project["github_issues"].(map[string]any)
	spec, _ := g["spec"].(string)
	got := issuesSettings{on: g["on"] == true, repositories: strings.Join(toStrings(g["repositories"]), ","), label: g["label"].(string), needMe: g["new_issues_need_me"] == true, spec: spec}
	if g == nil || got != want {
		t.Fatalf("github_issues = %v, want %+v", project["github_issues"], want)
	}
}

// acmeIssues is acme's GitHub Issues settings, as a Project read gives
// them.
func acmeIssues(t *testing.T, srv *codecentraltest.Server, want issuesSettings) {
	t.Helper()
	githubIssues(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), want)
}

func TestEveryProjectReadCarriesGitHubIssuesOff(t *testing.T) {
	srv := newServer(t)
	acmeIssues(t, srv, issuesSettings{})
	for _, row := range httpOK(t, srv, httpCall{"GET", "/api/projects", ""})["rows"].([]any) {
		githubIssues(t, row.(map[string]any), issuesSettings{})
	}
}

func TestSetGitHubIssuesIsRejectedForAnAgent(t *testing.T) {
	srv := newServer(t)
	status, b := httpDoAs(t, srv, core.Agent, setGitHubIssues(`{"on":true,"repositories":["acme/web"]}`))
	if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) || !strings.Contains(e.Message, "human") {
		t.Fatalf("status %d, want %d; message %q", status, httpapi.Status(core.ActionNotAllowed), e.Message)
	}
	acmeIssues(t, srv, issuesSettings{})
}

func TestTurningGitHubIssuesOnNeedsARepository(t *testing.T) {
	srv := newServer(t)
	for _, body := range []string{`{"on":true}`, `{"on":true,"repositories":["  "]}`} {
		e := httpRejected(t, srv, setGitHubIssues(body), core.Invalid)
		if e.Field != "repositories" || !strings.Contains(e.Message, "owner/name") {
			t.Fatalf("%s: field %q message %q, want repository asking for owner/name", body, e.Field, e.Message)
		}
	}
	// Off, the repository may stay empty, or be kept for later; no
	// Issues Spec is made until it is turned on.
	githubIssues(t, httpOK(t, srv, setGitHubIssues(`{"on":false,"repositories":["acme/web"],"label":"bug","full":true}`)),
		issuesSettings{repositories: "acme/web", label: "bug"})
}

func TestAGitHubIssuesRepositoryIsOwnerSlashName(t *testing.T) {
	srv := newServer(t)
	for _, repo := range []string{"web", "acme/", "acme/web/extra", "https://github.com/acme/web"} {
		e := httpRejected(t, srv, setGitHubIssues(`{"on":true,"repositories":["`+repo+`"]}`), core.Invalid)
		if e.Field != "repositories" || !strings.Contains(e.Message, repo) {
			t.Fatalf("%q: field %q message %q, want repository naming it", repo, e.Field, e.Message)
		}
	}
	acmeIssues(t, srv, issuesSettings{})
}

func TestAnArchivedProjectRejectsSetGitHubIssues(t *testing.T) {
	srv := newServer(t)
	human := core.WithActor(context.Background(), core.Human)
	must(t, func() error { _, err := srv.Core.ArchiveProject(human, "other"); return err })
	httpRejected(t, srv, httpCall{"POST", "/api/p/other/set_github_issues", `{"on":false}`}, core.ActionNotAllowed)
	httpRejected(t, srv, httpCall{"POST", "/api/p/nope/set_github_issues", `{"on":false}`}, core.UnknownProject)
}

// issuesSpec is acme's Spec id as get_spec reads it.
func issuesSpec(t *testing.T, srv *codecentraltest.Server, n int) map[string]any {
	t.Helper()
	return httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs/" + strconv.Itoa(n), ""})
}

// specCount is how many Specs acme has.
func specCount(t *testing.T, srv *codecentraltest.Server) int {
	t.Helper()
	return len(httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs", ""})["rows"].([]any))
}

func TestTurningGitHubIssuesOnCreatesTheIssuesSpecOnce(t *testing.T) {
	srv := newServer(t)
	before := specCount(t, srv)
	out := httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web"],"full":true}`))
	githubIssues(t, out, issuesSettings{on: true, repositories: "acme/web", spec: "acme-4"})
	s := issuesSpec(t, srv, 4)
	if s["title"] != "GitHub Issues: acme/web" || s["link"] != nil || fmt.Sprint(s["labels"]) != "[long-lived]" || s["requirement"] != nil || !strings.Contains(s["body"].(string), "Incoming Issue") {
		t.Fatalf("Issues Spec = %v, want a long-lived Spec titled for acme/web, with no Link and no Stages", s)
	}

	// Off and on again, and on with other settings, reuse it.
	httpOK(t, srv, setGitHubIssues(`{"on":false,"repositories":["acme/web"]}`))
	acmeIssues(t, srv, issuesSettings{repositories: "acme/web", spec: "acme-4"})
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web"]}`))
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web"],"label":"bug","new_issues_need_me":true}`))
	acmeIssues(t, srv, issuesSettings{on: true, repositories: "acme/web", label: "bug", needMe: true, spec: "acme-4"})
	if n := specCount(t, srv); n != before+1 {
		t.Fatalf("%d Specs, want %d: one Issues Spec", n, before+1)
	}
}

func TestChangingTheRepositoryRetitlesTheIssuesSpec(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web"]}`))
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/site"]}`))
	if s := issuesSpec(t, srv, 4); s["title"] != "GitHub Issues: acme/site" {
		t.Fatalf("title %v, want it to follow the repository", s["title"])
	}
	// Off, the repository may still change, and the title follows it.
	httpOK(t, srv, setGitHubIssues(`{"on":false,"repositories":["acme/app"]}`))
	if s := issuesSpec(t, srv, 4); s["title"] != "GitHub Issues: acme/app" {
		t.Fatalf("title %v, want it to follow the repository while off", s["title"])
	}
	// Off, the repository may be cleared; the next one still retitles it.
	httpOK(t, srv, setGitHubIssues(`{"on":false,"repositories":[]}`))
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/api"]}`))
	if s := issuesSpec(t, srv, 4); s["title"] != "GitHub Issues: acme/api" {
		t.Fatalf("title %v, want it to follow a repository set after clearing one", s["title"])
	}
}

func TestChangingTheRepositoryKeepsTheHumansTitle(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web"]}`))
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/specs/4/edit_spec", `{"title":"Bug reports"}`})
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/site"]}`))
	if s := issuesSpec(t, srv, 4); s["title"] != "Bug reports" {
		t.Fatalf("title %v, want the human's kept", s["title"])
	}
}

func TestTurningGitHubIssuesOffLeavesEveryLink(t *testing.T) {
	srv := newServer(t, linkedItems)
	links := func() []any {
		return []any{issuesSpec(t, srv, 4)["link"], httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/5", ""})["link"]}
	}
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web"]}`))
	before := links()
	httpOK(t, srv, setGitHubIssues(`{"on":false,"repositories":["acme/web"]}`))
	after := links()
	for i := range before {
		if before[i] == nil || !mapsEqual(before[i].(map[string]any), after[i].(map[string]any)) {
			t.Fatalf("links after turning off = %v, want %v", after, before)
		}
	}
}

func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// issuesChecks is a fake Issue Watcher check mode: it records each check
// and answers with result.
type issuesChecks struct {
	mu     sync.Mutex
	calls  []core.IssuesCheckInput
	result core.WatcherCheckResult
}

func (c *issuesChecks) check(_ context.Context, in core.IssuesCheckInput) core.WatcherCheckResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, in)
	return c.result
}

func (c *issuesChecks) ran() []core.IssuesCheckInput {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.calls)
}

func newIssuesCheckServer(t *testing.T, checks *issuesChecks) *codecentraltest.Server {
	t.Helper()
	return newServerWith(t, []codecentraltest.Option{codecentraltest.WithIssuesCheck(checks.check)})
}

func TestTurningGitHubIssuesOnRunsTheCheckWithTheGHPathOption(t *testing.T) {
	checks := &issuesChecks{}
	srv := newIssuesCheckServer(t, checks)
	gh := filepath.Join(t.TempDir(), "gh")
	httpOK(t, srv, httpCall{"POST", "/api/set_options", `{"gh_path":` + strconv.Quote(gh) + `}`})
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web"]}`))
	want := []core.IssuesCheckInput{{Repositories: []string{"acme/web"}, GHPath: gh}}
	if got := checks.ran(); !reflect.DeepEqual(got, want) {
		t.Fatalf("checks = %+v, want %+v", got, want)
	}
}

func TestAFailedIssuesCheckRefusesTurningGitHubIssuesOnWithItsFix(t *testing.T) {
	for _, problem := range []core.WatcherProblem{core.ProblemGHMissing, core.ProblemLoggedOut, core.ProblemRepositoryUnreadable,
		core.ProblemIssuesDisabled, core.ProblemNoTriage, core.ProblemCheckFailed} {
		t.Run(string(problem), func(t *testing.T) {
			checks := &issuesChecks{result: core.WatcherCheckResult{Problem: problem, Reason: "gh says no", Fix: "Run gh auth login."}}
			srv := newIssuesCheckServer(t, checks)
			before := specCount(t, srv)
			e := httpRejected(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web"]}`), core.Invalid)
			field := "on"
			if problem == core.ProblemRepositoryUnreadable || problem == core.ProblemIssuesDisabled || problem == core.ProblemNoTriage {
				field = "repositories"
			}
			if e.Field != field || e.Message != "gh says no" || e.Fix != "Run gh auth login." {
				t.Fatalf("refusal = %+v, want the check's reason and fix on %s", e, field)
			}
			acmeIssues(t, srv, issuesSettings{})
			if n := specCount(t, srv); n != before {
				t.Fatalf("a failed check made %d Specs", n-before)
			}
		})
	}
}

func TestOnlyTurningGitHubIssuesOnOrChangingItsRepositoryRunsTheCheck(t *testing.T) {
	checks := &issuesChecks{}
	srv := newIssuesCheckServer(t, checks)
	httpOK(t, srv, setGitHubIssues(`{"on":false,"repositories":["acme/web"]}`))
	if n := len(checks.ran()); n != 0 {
		t.Fatalf("leaving GitHub Issues off ran %d checks", n)
	}
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web"]}`))
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web"],"label":"bug","new_issues_need_me":true}`))
	if n := len(checks.ran()); n != 1 {
		t.Fatalf("turning on, then changing only the label and New issues need me, ran %d checks, want 1", n)
	}
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/site"]}`))
	if got := checks.ran(); len(got) != 2 || !slices.Equal(got[1].Repositories, []string{"acme/site"}) {
		t.Fatalf("checks = %+v, want a second for the new repository", got)
	}

	// Turning off runs no check, even when the check would fail.
	checks.result = core.WatcherCheckResult{Problem: core.ProblemLoggedOut, Reason: "logged out", Fix: "Run gh auth login."}
	githubIssues(t, httpOK(t, srv, setGitHubIssues(`{"on":false,"repositories":["acme/site"],"full":true}`)),
		issuesSettings{repositories: "acme/site", spec: "acme-4"})
	if n := len(checks.ran()); n != 2 {
		t.Fatalf("turning off ran a check: %d checks", n)
	}
}

func TestTheSameGitHubIssuesSettingsWriteNothing(t *testing.T) {
	srv := newServer(t)
	if out := httpOK(t, srv, setGitHubIssues(`{"on":false}`)); len(out["changed"].([]any)) != 0 {
		t.Fatalf("the default settings changed %v", out["changed"])
	}
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web"]}`))
	if out := httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":[" acme/web"]}`)); len(out["changed"].([]any)) != 0 {
		t.Fatalf("the same settings changed %v", out["changed"])
	}
}

// toStrings is a JSON array of strings, as decoded into any.
func toStrings(v any) []string {
	var out []string
	for _, s := range v.([]any) {
		out = append(out, s.(string))
	}
	return out
}

// Listing more than one repository titles the Issues Spec "GitHub
// Issues", and listing just one again titles it for that repository.
func TestMoreThanOneRepositoryTitlesTheIssuesSpecGitHubIssues(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web"]}`))
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web","acme/api"]}`))
	if s := issuesSpec(t, srv, 4); s["title"] != "GitHub Issues" {
		t.Fatalf("title %v, want GitHub Issues once two repositories are listed", s["title"])
	}
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/api"]}`))
	if s := issuesSpec(t, srv, 4); s["title"] != "GitHub Issues: acme/api" {
		t.Fatalf("title %v, want it named for the one repository left", s["title"])
	}

	// A Project that lists several from the start makes the Spec with the
	// short title, and the human's own title is kept.
	other := newServer(t)
	httpOK(t, other, setGitHubIssues(`{"on":true,"repositories":["acme/web","acme/api"]}`))
	if s := issuesSpec(t, other, 4); s["title"] != "GitHub Issues" {
		t.Fatalf("title %v, want GitHub Issues", s["title"])
	}
	httpOK(t, other, httpCall{"POST", "/api/p/acme/specs/4/edit_spec", `{"title":"Bug reports"}`})
	httpOK(t, other, setGitHubIssues(`{"on":true,"repositories":["acme/web"]}`))
	if s := issuesSpec(t, other, 4); s["title"] != "Bug reports" {
		t.Fatalf("title %v, want the human's kept", s["title"])
	}
}

// Each repository is owner/name and listed once; the filter label and New
// issues need me are the one setting for every repository.
func TestEachListedRepositoryIsOwnerSlashNameAndListedOnce(t *testing.T) {
	srv := newServer(t)
	e := httpRejected(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web","acme/api","web"]}`), core.Invalid)
	if e.Field != "repositories" || !strings.Contains(e.Message, `"web"`) {
		t.Fatalf("refusal = %+v, want the malformed repository named", e)
	}
	e = httpRejected(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web","ACME/Web"]}`), core.Invalid)
	if e.Field != "repositories" || !strings.Contains(e.Message, "listed twice") {
		t.Fatalf("refusal = %+v, want a repository listed twice refused", e)
	}
	githubIssues(t, httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web","acme/api"],"label":"bug","new_issues_need_me":true,"full":true}`)),
		issuesSettings{on: true, repositories: "acme/web,acme/api", label: "bug", needMe: true, spec: "acme-4"})
}

// Only a repository newly listed is checked, and a failing one is
// refused with the check's reason and fix, on repositories.
func TestOnlyANewlyListedRepositoryIsChecked(t *testing.T) {
	checks := &issuesChecks{}
	srv := newIssuesCheckServer(t, checks)
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web"]}`))
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web","acme/api","acme/site"]}`))
	if got, want := checks.ran(), []core.IssuesCheckInput{{Repositories: []string{"acme/web"}}, {Repositories: []string{"acme/api", "acme/site"}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("checks = %+v, want %+v", got, want)
	}
	// Removing one runs no check.
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web","acme/site"]}`))
	if n := len(checks.ran()); n != 2 {
		t.Fatalf("removing a repository ran a check: %d checks", n)
	}

	checks.result = core.WatcherCheckResult{Problem: core.ProblemNoTriage, Reason: "acme/new: no triage", Fix: "acme/new: grant triage",
		Repositories: []core.RepositoryProblem{{Repository: "acme/new", Problem: core.ProblemNoTriage, Message: "no triage", Fix: "grant triage"}}}
	e := httpRejected(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web","acme/site","acme/new"]}`), core.Invalid)
	if e.Field != "repositories" || e.Message != "acme/new: no triage" || e.Fix != "acme/new: grant triage" {
		t.Fatalf("refusal = %+v, want the failing repository's reason and fix on repositories", e)
	}
	acmeIssues(t, srv, issuesSettings{on: true, repositories: "acme/web,acme/site", spec: "acme-4"})
}
