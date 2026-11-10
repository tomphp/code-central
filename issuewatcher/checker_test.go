package issuewatcher_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tomoram/code-central/ciwatcher/ciwatchertest"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fakegh"
	"github.com/tomoram/code-central/issuewatcher"
)

func TestMain(m *testing.M) { ciwatchertest.Main(m) }

var ctx = context.Background()

// serviceWith is a real Code Central with a Project acme whose Issues
// check runs the test binary as code-central, as the service runs its
// own executable, with gh found on path. Its client is the human's.
func serviceWith(t *testing.T, h *ciwatchertest.Harness, path ...string) *codecentraltest.Server {
	t.Helper()
	return serviceRunning(t, h.CodeCentral, h.Environ(path...))
}

// serviceRunning is serviceWith with program, in environ, as the
// service's own executable.
func serviceRunning(t *testing.T, program string, environ []string) *codecentraltest.Server {
	t.Helper()
	srv := codecentraltest.NewServer(t, codecentraltest.WithIssuesCheck(issuewatcher.Checker(program, environ)))
	if _, err := srv.Client.CreateProject(ctx, core.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	srv.Client.Actor = core.Human
	return srv
}

// repository makes gh read acme/web as GitHub answers for a repository
// with issues on or off, on which gh's login has permissions.
func repository(h *ciwatchertest.Harness, issues bool, permissions string) {
	h.Answer(fakegh.Answer{Args: []string{"api", "repos/acme/web"},
		Stdout: `{"full_name":"acme/web","has_issues":` + strconv.FormatBool(issues) + `,"permissions":` + permissions + `}`})
}

const (
	triage = `{"admin":false,"maintain":false,"push":false,"triage":true,"pull":true}`
	write  = `{"admin":false,"maintain":false,"push":true,"triage":true,"pull":true}`
	read   = `{"admin":false,"maintain":false,"push":false,"triage":false,"pull":true}`
)

func turnOn(srv *codecentraltest.Server) error {
	_, err := srv.Client.SetGitHubIssues(ctx, "acme", core.SetGitHubIssuesInput{On: true, Repository: "acme/web"})
	return err
}

// refusal is err as the refusal of turning GitHub Issues on.
func refusal(t *testing.T, err error) *core.Error {
	t.Helper()
	var e *core.Error
	if !errors.As(err, &e) || e.Code != core.Invalid || (e.Field != "on" && e.Field != "repository") {
		t.Fatalf("err = %v, want GitHub Issues refused", err)
	}
	return e
}

// settings is acme's GitHub Issues settings, as the HTTP API reads them.
func settings(t *testing.T, srv *codecentraltest.Server) core.GitHubIssuesView {
	t.Helper()
	p, err := srv.Client.GetProject(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	return p.GitHubIssues
}

func TestGitHubIssuesTurnsOnWhenTheCheckPasses(t *testing.T) {
	for name, permissions := range map[string]string{"triage": triage, "write": write} {
		t.Run(name, func(t *testing.T) {
			h := ciwatchertest.New(t)
			repository(h, true, permissions)
			srv := serviceWith(t, h, h.Bin)
			if err := turnOn(srv); err != nil {
				t.Fatalf("turning on: %v", err)
			}
			if g := settings(t, srv); !g.On || g.Spec == "" {
				t.Fatalf("settings = %+v, want on with its Issues Spec", g)
			}
			var got [][]string
			for _, c := range h.Calls() {
				got = append(got, c.Args)
			}
			want := [][]string{{"auth", "status", "--hostname", "github.com"}, {"api", "repos/acme/web", "--include"}}
			if !slices.EqualFunc(got, want, slices.Equal) {
				t.Fatalf("gh calls = %q, want %q", got, want)
			}
		})
	}
}

func TestEachFailedCheckRefusesGitHubIssuesWithItsFix(t *testing.T) {
	// gh missing is a gh path Option with no gh there: with none, the
	// check would look in the real install places, where this machine
	// may have gh.
	noGH := filepath.Join(t.TempDir(), "gh")
	for _, c := range []struct {
		name        string
		setup       func(h *ciwatchertest.Harness, srv *codecentraltest.Server)
		problem     core.WatcherProblem
		reason, fix string
	}{
		{"gh missing", func(_ *ciwatchertest.Harness, srv *codecentraltest.Server) {
			if _, err := srv.Client.SetOptions(ctx, core.SetOptionsInput{GHPath: &noGH}); err != nil {
				t.Fatal(err)
			}
		}, core.ProblemGHMissing, "gh isn't there", "gh path Option"},
		{"logged out", func(h *ciwatchertest.Harness, _ *codecentraltest.Server) { h.LoggedOut() }, core.ProblemLoggedOut, "isn't logged in", "gh auth login"},
		{"no read access", func(h *ciwatchertest.Harness, _ *codecentraltest.Server) { h.Unreadable("acme/web") }, core.ProblemRepositoryUnreadable,
			"can't read the repository acme/web", "owner/name"},
		{"issues disabled", func(h *ciwatchertest.Harness, _ *codecentraltest.Server) { repository(h, false, write) }, core.ProblemIssuesDisabled,
			"acme/web has its issues turned off", "Features"},
		{"below triage", func(h *ciwatchertest.Harness, _ *codecentraltest.Server) { repository(h, true, read) }, core.ProblemNoTriage,
			"can't triage the issues of acme/web", "triage permission"},
		{"no permissions", func(h *ciwatchertest.Harness, _ *codecentraltest.Server) { repository(h, true, "null") }, core.ProblemNoTriage,
			"can't triage the issues of acme/web", "triage permission"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := ciwatchertest.New(t)
			repository(h, true, triage)
			srv := serviceWith(t, h, h.Bin)
			c.setup(h, srv)
			e := refusal(t, turnOn(srv))
			if !strings.Contains(e.Message, c.reason) || !strings.Contains(e.Fix, c.fix) {
				t.Fatalf("refusal %q, fix %q; want %q and %q", e.Message, e.Fix, c.reason, c.fix)
			}
			if g := settings(t, srv); g.On || g.Spec != "" {
				t.Fatalf("a failed check left %+v, want GitHub Issues off with no Issues Spec", g)
			}
		})
	}
}

func TestAnAnswerTheCheckCantReadRefusesGitHubIssues(t *testing.T) {
	h := ciwatchertest.New(t)
	h.Answer(fakegh.Answer{Args: []string{"api", "repos/acme/web"}, Stdout: "not json"})
	srv := serviceWith(t, h, h.Bin)
	if e := refusal(t, turnOn(srv)); e.Field != "on" || !strings.Contains(e.Message, "acme/web") || e.Fix == "" {
		t.Fatalf("refusal %+v, want the check failed, naming the repository, with a fix", e)
	}
}

func TestACheckThatDoesntFinishRefusesGitHubIssues(t *testing.T) {
	h := ciwatchertest.New(t)
	srv := serviceRunning(t, filepath.Join(t.TempDir(), "missing"), h.Environ(h.Bin))
	if e := refusal(t, turnOn(srv)); !strings.Contains(e.Message, "Issue Watcher's check didn't finish") || !strings.Contains(e.Fix, "GitHub Issues") {
		t.Fatalf("refusal %+v, want the check that didn't finish, with a fix", e)
	}
}

func TestARateLimitedCheckRefusesGitHubIssuesUntilItEnds(t *testing.T) {
	h := ciwatchertest.New(t)
	h.Answer(fakegh.Answer{Args: []string{"api", "repos/acme/web"}, Exit: 1,
		Stdout: "HTTP/2.0 403 Forbidden\r\nX-Ratelimit-Remaining: 0\r\nX-Ratelimit-Reset: 4102444800\r\n\r\n{}", Stderr: "gh: API rate limit exceeded (HTTP 403)\n"})
	srv := serviceWith(t, h, h.Bin)
	if e := refusal(t, turnOn(srv)); e.Field != "on" || !strings.Contains(e.Message, "rate limit") || !strings.Contains(e.Fix, "Try again after") {
		t.Fatalf("refusal %+v, want the rate limit, with when to try again", e)
	}
}
