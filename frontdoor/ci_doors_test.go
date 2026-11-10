package frontdoor_test

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/httpapi"
)

// CI settings (code-central-251): set_ci_tracking is the human's, so HTTP
// only; report_push_remote is the Factory's, over HTTP only.
func init() {
	cases["set_ci_tracking"] = opCase{
		ok: call{http: setCITracking(`{"tracking":true,"repository":" acme/web ","ignored_workflows":[" nightly.yml ",""]}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "ci")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			field("key", "acme")(t, out)
			ciSettings(t, out, true, "acme/web", []string{"nightly.yml"}, "")
		},
		bad:  call{http: setCITracking(`{"tracking":true}`)},
		code: core.Invalid,
	}
	cases["report_push_remote"] = opCase{
		ok: call{http: reportPushRemote(`{"url":"git@github.com:acme/web.git"}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "ci")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			ciSettings(t, out, false, "", nil, "git@github.com:acme/web.git")
		},
		bad:  call{http: reportPushRemote(`{"url":" "}`)},
		code: core.Invalid,
	}
}

func setCITracking(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/set_ci_tracking", body}
}

func reportPushRemote(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/report_push_remote", body}
}

// ciSettings checks a Project's ci object.
func ciSettings(t *testing.T, project map[string]any, tracking bool, repository string, ignored []string, pushRemoteURL string) {
	t.Helper()
	c, _ := project["ci"].(map[string]any)
	var gotIgnored []string
	list, isList := c["ignored_workflows"].([]any)
	for _, w := range list {
		gotIgnored = append(gotIgnored, w.(string))
	}
	if c == nil || !isList || c["tracking"] != tracking || c["provider"] != "github_actions" || c["repository"] != repository ||
		!slices.Equal(gotIgnored, ignored) || c["push_remote_url"] != pushRemoteURL {
		t.Fatalf("ci = %v, want tracking %v, github_actions, repository %q, ignored %v, push remote %q", project["ci"], tracking, repository, ignored, pushRemoteURL)
	}
}

func TestEveryProjectReadCarriesCITrackingOff(t *testing.T) {
	srv := newServer(t)
	ciSettings(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), false, "", nil, "")
	for _, row := range httpOK(t, srv, httpCall{"GET", "/api/projects", ""})["rows"].([]any) {
		ciSettings(t, row.(map[string]any), false, "", nil, "")
	}
}

func TestSetCITrackingIsReadBackByEveryProjectRead(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, setCITracking(`{"tracking":true,"provider":"github_actions","repository":"acme/web","ignored_workflows":["nightly.yml","preview.yaml"]}`))
	ciSettings(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), true, "acme/web", []string{"nightly.yml", "preview.yaml"}, "")
	rows := httpOK(t, srv, httpCall{"GET", "/api/projects", ""})["rows"].([]any)
	ciSettings(t, rows[0].(map[string]any), true, "acme/web", []string{"nightly.yml", "preview.yaml"}, "")
	ciSettings(t, rows[1].(map[string]any), false, "", nil, "")
}

func TestSetCITrackingIsRejectedForAnAgent(t *testing.T) {
	srv := newServer(t)
	status, b := httpDoAs(t, srv, core.Agent, setCITracking(`{"tracking":true,"repository":"acme/web"}`))
	if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) || !strings.Contains(e.Message, "human") {
		t.Fatalf("status %d, want %d; message %q", status, httpapi.Status(core.ActionNotAllowed), e.Message)
	}
	ciSettings(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), false, "", nil, "")
}

func TestTurningCITrackingOnNeedsARepository(t *testing.T) {
	srv := newServer(t)
	for _, body := range []string{`{"tracking":true}`, `{"tracking":true,"repository":"  "}`} {
		e := httpRejected(t, srv, setCITracking(body), core.Invalid)
		if e.Field != "repository" || !strings.Contains(e.Message, "owner/name") {
			t.Fatalf("%s: field %q message %q, want repository asking for owner/name", body, e.Field, e.Message)
		}
	}
	// Off, the repository may stay empty, or be kept for later.
	ciSettings(t, httpOK(t, srv, setCITracking(`{"tracking":false,"full":true}`)), false, "", nil, "")
	ciSettings(t, httpOK(t, srv, setCITracking(`{"tracking":false,"repository":"acme/web","full":true}`)), false, "acme/web", nil, "")
}

func TestARepositoryIsOwnerSlashName(t *testing.T) {
	srv := newServer(t)
	for _, repo := range []string{"web", "acme/", "/web", "acme/web/extra", "https://github.com/acme/web", "acme /web"} {
		e := httpRejected(t, srv, setCITracking(`{"tracking":true,"repository":"`+repo+`"}`), core.Invalid)
		if e.Field != "repository" || !strings.Contains(e.Message, repo) {
			t.Fatalf("%q: field %q message %q, want repository naming it", repo, e.Field, e.Message)
		}
	}
	ciSettings(t, httpOK(t, srv, setCITracking(`{"tracking":true,"repository":"\tacme-co/web.site ","full":true}`)), true, "acme-co/web.site", nil, "")
}

func TestIgnoredWorkflowsAreWorkflowFileNames(t *testing.T) {
	srv := newServer(t)
	for _, w := range []string{".github/workflows/ci.yml", "ci", "CI build"} {
		e := httpRejected(t, srv, setCITracking(`{"tracking":false,"ignored_workflows":["`+w+`"]}`), core.Invalid)
		if e.Field != "ignored_workflows" || !strings.Contains(e.Message, w) || !strings.Contains(e.Message, "ci.yml") {
			t.Fatalf("%q: field %q message %q, want ignored_workflows naming it and an example", w, e.Field, e.Message)
		}
	}
	out := httpOK(t, srv, setCITracking(`{"tracking":false,"ignored_workflows":[" nightly.yml","","deploy.yaml","nightly.yml"],"full":true}`))
	ciSettings(t, out, false, "", []string{"nightly.yml", "deploy.yaml"}, "")
}

func TestAnUnknownCIProviderIsInvalid(t *testing.T) {
	srv := newServer(t)
	e := httpRejected(t, srv, setCITracking(`{"tracking":true,"provider":"circleci","repository":"acme/web"}`), core.Invalid)
	if e.Field != "provider" || !strings.Contains(e.Message, `"github_actions"`) {
		t.Fatalf("field %q message %q, want provider naming github_actions", e.Field, e.Message)
	}
}

func TestAnArchivedProjectRejectsSetCITracking(t *testing.T) {
	srv := newServer(t)
	human := core.WithActor(context.Background(), core.Human)
	must(t, func() error { _, err := srv.Core.ArchiveProject(human, "other"); return err })
	httpRejected(t, srv, httpCall{"POST", "/api/p/other/set_ci_tracking", `{"tracking":false}`}, core.ActionNotAllowed)
	httpRejected(t, srv, httpCall{"POST", "/api/p/nope/set_ci_tracking", `{"tracking":false}`}, core.UnknownProject)
}

// Every store write sends a notice, so none means nothing was written.
func TestTheSameCISettingsWriteNothingAndAChangeSendsANotice(t *testing.T) {
	srv := newServer(t)
	notices, stop := srv.Core.Events().Subscribe()
	defer stop()
	if out := httpOK(t, srv, setCITracking(`{"tracking":false,"provider":"github_actions"}`)); len(out["changed"].([]any)) != 0 {
		t.Fatalf("the default settings changed %v", out["changed"])
	}
	httpOK(t, srv, setCITracking(`{"tracking":true,"repository":"acme/web"}`))
	select {
	case got := <-notices:
		if want := (events.Event{Type: events.ProjectChanged, Project: "acme"}); got != want {
			t.Fatalf("notice %+v, want %+v: the same values sent a notice", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a real change sent no notice")
	}
	httpOK(t, srv, setCITracking(`{"tracking":true,"repository":" acme/web"}`))
	select {
	case got := <-notices:
		t.Fatalf("the same values sent %+v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestTheFactoryReportsThePushRemoteURL(t *testing.T) {
	srv := newServer(t)
	status, b := httpDoAs(t, srv, core.Agent, reportPushRemote(`{"url":" https://github.com/acme/web.git "}`))
	if status != 200 {
		t.Fatalf("status %d: %s", status, b)
	}
	ciSettings(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), false, "", nil, "https://github.com/acme/web.git")

	// The human's settings keep the reported URL, and a new report keeps
	// the human's settings.
	httpOK(t, srv, setCITracking(`{"tracking":true,"repository":"acme/web"}`))
	httpDoAs(t, srv, core.Agent, reportPushRemote(`{"url":"git@github.com:acme/web.git"}`))
	ciSettings(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), true, "acme/web", nil, "git@github.com:acme/web.git")

	e := httpRejected(t, srv, reportPushRemote(`{"url":""}`), core.Invalid)
	if e.Field != "url" {
		t.Fatalf("field %q, want url", e.Field)
	}
}

// A Push remote URL may carry a login; Code Central keeps none.
func TestAPushRemoteURLIsStoredWithoutItsLogin(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, reportPushRemote(`{"url":"https://someone:ghp_secret@github.com/acme/web.git"}`))
	ciSettings(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), false, "", nil, "https://github.com/acme/web.git")
	httpOK(t, srv, reportPushRemote(`{"url":"ssh://git@github.com/acme/web.git"}`))
	ciSettings(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), false, "", nil, "ssh://git@github.com/acme/web.git")
}

func TestTheSamePushRemoteURLWritesNothing(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, reportPushRemote(`{"url":"git@github.com:acme/web.git"}`))
	notices, stop := srv.Core.Events().Subscribe()
	defer stop()
	if out := httpOK(t, srv, reportPushRemote(`{"url":"git@github.com:acme/web.git"}`)); len(out["changed"].([]any)) != 0 {
		t.Fatalf("the same URL changed %v", out["changed"])
	}
	select {
	case got := <-notices:
		t.Fatalf("the same URL sent %+v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

// ciChecks is a fake CI Watcher check mode: it records each check and
// answers with result.
type ciChecks struct {
	mu     sync.Mutex
	calls  []core.CICheckInput
	result core.CICheckResult
}

func (c *ciChecks) check(_ context.Context, in core.CICheckInput) core.CICheckResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, in)
	return c.result
}

func (c *ciChecks) ran() []core.CICheckInput {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.calls)
}

// newCICheckServer is newServer with checks as its CI Watcher's check
// mode.
func newCICheckServer(t *testing.T, checks *ciChecks) *codecentraltest.Server {
	t.Helper()
	return newServerWith(t, []codecentraltest.Option{codecentraltest.WithCICheck(checks.check)})
}

func TestTurningCITrackingOnRunsTheCheckWithTheGHPathOption(t *testing.T) {
	checks := &ciChecks{}
	srv := newCICheckServer(t, checks)
	gh := filepath.Join(t.TempDir(), "gh")
	httpOK(t, srv, httpCall{"POST", "/api/set_options", `{"gh_path":` + strconv.Quote(gh) + `}`})
	ciSettings(t, httpOK(t, srv, setCITracking(`{"tracking":true,"repository":"acme/web","full":true}`)), true, "acme/web", nil, "")
	want := []core.CICheckInput{{Provider: "github_actions", Repository: "acme/web", GHPath: gh}}
	if got := checks.ran(); !slices.Equal(got, want) {
		t.Fatalf("checks = %+v, want %+v", got, want)
	}
}

func TestAFailedCICheckRefusesTurningCITrackingOnWithItsFix(t *testing.T) {
	for _, problem := range []core.CIProblem{core.CIGHMissing, core.CILoggedOut, core.CIRepositoryUnreadable, core.CICheckFailed} {
		t.Run(string(problem), func(t *testing.T) {
			checks := &ciChecks{result: core.CICheckResult{Problem: problem, Reason: "gh says no", Fix: "Run gh auth login."}}
			srv := newCICheckServer(t, checks)
			e := httpRejected(t, srv, setCITracking(`{"tracking":true,"repository":"acme/web"}`), core.Invalid)
			if e.Field != "tracking" || e.Message != "gh says no" || e.Fix != "Run gh auth login." {
				t.Fatalf("refusal = %+v, want the check's reason and fix on tracking", e)
			}
			ciSettings(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), false, "", nil, "")
		})
	}
}

func TestOnlyTurningCITrackingOnOrChangingItsRepositoryRunsTheCheck(t *testing.T) {
	checks := &ciChecks{}
	srv := newCICheckServer(t, checks)
	httpOK(t, srv, setCITracking(`{"tracking":false,"repository":"acme/web"}`))
	if n := len(checks.ran()); n != 0 {
		t.Fatalf("leaving CI Tracking off ran %d checks", n)
	}
	httpOK(t, srv, setCITracking(`{"tracking":true,"repository":"acme/web"}`))
	httpOK(t, srv, setCITracking(`{"tracking":true,"repository":"acme/web","ignored_workflows":["nightly.yml"]}`))
	if n := len(checks.ran()); n != 1 {
		t.Fatalf("turning on, then changing only Ignored workflows, ran %d checks, want 1", n)
	}
	httpOK(t, srv, setCITracking(`{"tracking":true,"repository":"acme/site"}`))
	if got := checks.ran(); len(got) != 2 || got[1].Repository != "acme/site" {
		t.Fatalf("checks = %+v, want a second for the new repository", got)
	}

	// Turning off runs no check, even when the check would fail.
	checks.result = core.CICheckResult{Problem: core.CILoggedOut, Reason: "logged out", Fix: "Run gh auth login."}
	ciSettings(t, httpOK(t, srv, setCITracking(`{"tracking":false,"repository":"acme/site","full":true}`)), false, "acme/site", nil, "")
	if n := len(checks.ran()); n != 2 {
		t.Fatalf("turning off ran a check: %d checks", n)
	}
}

func TestAnAgentsSetCITrackingRunsNoCheck(t *testing.T) {
	checks := &ciChecks{}
	srv := newCICheckServer(t, checks)
	httpDoAs(t, srv, core.Agent, setCITracking(`{"tracking":true,"repository":"acme/web"}`))
	if n := len(checks.ran()); n != 0 {
		t.Fatalf("an agent's refused set_ci_tracking ran %d checks", n)
	}
}
