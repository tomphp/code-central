package frontdoor_test

import (
	"context"
	"path/filepath"
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
		ok: call{http: setGitHubIssues(`{"on":true,"repository":" acme/web ","label":" triage ","new_issues_need_me":true}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "github_issues")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			field("key", "acme")(t, out)
			githubIssues(t, out, issuesSettings{on: true, repository: "acme/web", label: "triage", needMe: true, spec: "acme-4"})
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
	on         bool
	repository string
	label      string
	needMe     bool
	spec       string
}

// githubIssues checks a Project's github_issues object is want.
func githubIssues(t *testing.T, project map[string]any, want issuesSettings) {
	t.Helper()
	g, _ := project["github_issues"].(map[string]any)
	spec, _ := g["spec"].(string)
	got := issuesSettings{on: g["on"] == true, repository: g["repository"].(string), label: g["label"].(string), needMe: g["new_issues_need_me"] == true, spec: spec}
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
	status, b := httpDoAs(t, srv, core.Agent, setGitHubIssues(`{"on":true,"repository":"acme/web"}`))
	if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) || !strings.Contains(e.Message, "human") {
		t.Fatalf("status %d, want %d; message %q", status, httpapi.Status(core.ActionNotAllowed), e.Message)
	}
	acmeIssues(t, srv, issuesSettings{})
}

func TestTurningGitHubIssuesOnNeedsARepository(t *testing.T) {
	srv := newServer(t)
	for _, body := range []string{`{"on":true}`, `{"on":true,"repository":"  "}`} {
		e := httpRejected(t, srv, setGitHubIssues(body), core.Invalid)
		if e.Field != "repository" || !strings.Contains(e.Message, "owner/name") {
			t.Fatalf("%s: field %q message %q, want repository asking for owner/name", body, e.Field, e.Message)
		}
	}
	// Off, the repository may stay empty, or be kept for later; no
	// Issues Spec is made until it is turned on.
	githubIssues(t, httpOK(t, srv, setGitHubIssues(`{"on":false,"repository":"acme/web","label":"bug","full":true}`)),
		issuesSettings{repository: "acme/web", label: "bug"})
}

func TestAGitHubIssuesRepositoryIsOwnerSlashName(t *testing.T) {
	srv := newServer(t)
	for _, repo := range []string{"web", "acme/", "acme/web/extra", "https://github.com/acme/web"} {
		e := httpRejected(t, srv, setGitHubIssues(`{"on":true,"repository":"`+repo+`"}`), core.Invalid)
		if e.Field != "repository" || !strings.Contains(e.Message, repo) {
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
	out := httpOK(t, srv, setGitHubIssues(`{"on":true,"repository":"acme/web","full":true}`))
	githubIssues(t, out, issuesSettings{on: true, repository: "acme/web", spec: "acme-4"})
	s := issuesSpec(t, srv, 4)
	if s["title"] != "GitHub Issues: acme/web" || s["link"] != nil || len(s["labels"].([]any)) != 0 || s["requirement"] != nil || !strings.Contains(s["body"].(string), "Incoming Issue") {
		t.Fatalf("Issues Spec = %v, want an ordinary Spec titled for acme/web, with no Link and no Stages", s)
	}

	// Off and on again, and on with other settings, reuse it.
	httpOK(t, srv, setGitHubIssues(`{"on":false,"repository":"acme/web"}`))
	acmeIssues(t, srv, issuesSettings{repository: "acme/web", spec: "acme-4"})
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repository":"acme/web"}`))
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repository":"acme/web","label":"bug","new_issues_need_me":true}`))
	acmeIssues(t, srv, issuesSettings{on: true, repository: "acme/web", label: "bug", needMe: true, spec: "acme-4"})
	if n := specCount(t, srv); n != before+1 {
		t.Fatalf("%d Specs, want %d: one Issues Spec", n, before+1)
	}
}

func TestChangingTheRepositoryRetitlesTheIssuesSpec(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repository":"acme/web"}`))
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repository":"acme/site"}`))
	if s := issuesSpec(t, srv, 4); s["title"] != "GitHub Issues: acme/site" {
		t.Fatalf("title %v, want it to follow the repository", s["title"])
	}
	// Off, the repository may still change, and the title follows it.
	httpOK(t, srv, setGitHubIssues(`{"on":false,"repository":"acme/app"}`))
	if s := issuesSpec(t, srv, 4); s["title"] != "GitHub Issues: acme/app" {
		t.Fatalf("title %v, want it to follow the repository while off", s["title"])
	}
}

func TestChangingTheRepositoryKeepsTheHumansTitle(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repository":"acme/web"}`))
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/specs/4/edit_spec", `{"title":"Bug reports"}`})
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repository":"acme/site"}`))
	if s := issuesSpec(t, srv, 4); s["title"] != "Bug reports" {
		t.Fatalf("title %v, want the human's kept", s["title"])
	}
}

func TestTurningGitHubIssuesOffLeavesEveryLink(t *testing.T) {
	srv := newServer(t, linkedItems)
	links := func() []any {
		return []any{issuesSpec(t, srv, 4)["link"], httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/5", ""})["link"]}
	}
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repository":"acme/web"}`))
	before := links()
	httpOK(t, srv, setGitHubIssues(`{"on":false,"repository":"acme/web"}`))
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
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repository":"acme/web"}`))
	want := []core.IssuesCheckInput{{Repository: "acme/web", GHPath: gh}}
	if got := checks.ran(); !slices.Equal(got, want) {
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
			e := httpRejected(t, srv, setGitHubIssues(`{"on":true,"repository":"acme/web"}`), core.Invalid)
			field := "on"
			if problem == core.ProblemRepositoryUnreadable || problem == core.ProblemIssuesDisabled || problem == core.ProblemNoTriage {
				field = "repository"
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
	httpOK(t, srv, setGitHubIssues(`{"on":false,"repository":"acme/web"}`))
	if n := len(checks.ran()); n != 0 {
		t.Fatalf("leaving GitHub Issues off ran %d checks", n)
	}
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repository":"acme/web"}`))
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repository":"acme/web","label":"bug","new_issues_need_me":true}`))
	if n := len(checks.ran()); n != 1 {
		t.Fatalf("turning on, then changing only the label and New issues need me, ran %d checks, want 1", n)
	}
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repository":"acme/site"}`))
	if got := checks.ran(); len(got) != 2 || got[1].Repository != "acme/site" {
		t.Fatalf("checks = %+v, want a second for the new repository", got)
	}

	// Turning off runs no check, even when the check would fail.
	checks.result = core.WatcherCheckResult{Problem: core.ProblemLoggedOut, Reason: "logged out", Fix: "Run gh auth login."}
	githubIssues(t, httpOK(t, srv, setGitHubIssues(`{"on":false,"repository":"acme/site","full":true}`)),
		issuesSettings{repository: "acme/site", spec: "acme-4"})
	if n := len(checks.ran()); n != 2 {
		t.Fatalf("turning off ran a check: %d checks", n)
	}
}

func TestTheSameGitHubIssuesSettingsWriteNothing(t *testing.T) {
	srv := newServer(t)
	if out := httpOK(t, srv, setGitHubIssues(`{"on":false}`)); len(out["changed"].([]any)) != 0 {
		t.Fatalf("the default settings changed %v", out["changed"])
	}
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repository":"acme/web"}`))
	if out := httpOK(t, srv, setGitHubIssues(`{"on":true,"repository":" acme/web"}`)); len(out["changed"].([]any)) != 0 {
		t.Fatalf("the same settings changed %v", out["changed"])
	}
}
