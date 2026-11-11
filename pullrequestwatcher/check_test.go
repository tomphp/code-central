package pullrequestwatcher_test

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/ciwatcher/ciwatchertest"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fakegh"
	"github.com/tomoram/code-central/pullrequestwatcher"
	"github.com/tomoram/code-central/store"
)

// checking is a real Code Central whose Pull Request check runs the test
// binary as code-central, as the service runs its own executable, with gh
// found on path, and whose acme reported a GitHub Push remote.
func checking(t *testing.T, h *ciwatchertest.Harness, path ...string) *codecentraltest.Server {
	t.Helper()
	srv := codecentraltest.NewServer(t, codecentraltest.WithPullRequestCheck(pullrequestwatcher.Checker(h.CodeCentral, h.Environ(path...))))
	must(t, func() error {
		_, err := srv.Core.CreateProject(human, core.CreateProjectInput{Key: "acme"})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: "git@github.com:acme/web.git", Base: "main"})
		return err
	})
	return srv
}

func turnOn(srv *codecentraltest.Server) error {
	_, err := srv.Core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest})
	return err
}

func method(t *testing.T, srv *codecentraltest.Server) store.LandingMethod {
	t.Helper()
	p, err := srv.Client.GetProject(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	return p.Landing.Method
}

func TestPullRequestTurnsOnWhenTheCheckPassesAndCreatesNoPullRequest(t *testing.T) {
	h := ciwatchertest.New(t)
	h.Answer(fakegh.Answer{Args: []string{"api", repoPath}, Stdout: write})
	srv := checking(t, h, h.Bin)
	if err := turnOn(srv); err != nil {
		t.Fatalf("turning on: %v", err)
	}
	if got := method(t, srv); got != store.LandingPullRequest {
		t.Fatalf("method = %s, want pull_request", got)
	}
	var got [][]string
	for _, c := range h.Calls() {
		got = append(got, c.Args)
	}
	want := [][]string{{"auth", "status", "--hostname", "github.com"}, {"api", repoPath, "--include"}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("gh calls = %q, want only reads, %q", got, want)
	}
}

func TestEachFailedCheckRefusesPullRequestWithItsReasonAndFix(t *testing.T) {
	// gh missing is a gh path Option with no gh there: with none, the
	// check would look in the real install places.
	noGH := filepath.Join(t.TempDir(), "gh")
	for _, c := range []struct {
		name        string
		setup       func(h *ciwatchertest.Harness, srv *codecentraltest.Server)
		reason, fix string
	}{
		{"gh missing", func(_ *ciwatchertest.Harness, srv *codecentraltest.Server) {
			if _, err := srv.Core.SetOptions(human, core.SetOptionsInput{GHPath: &noGH}); err != nil {
				t.Fatal(err)
			}
		}, "gh isn't there", "gh path Option"},
		{"logged out", func(h *ciwatchertest.Harness, _ *codecentraltest.Server) { h.LoggedOut() }, "isn't logged in", "gh auth login"},
		{"no read access", func(h *ciwatchertest.Harness, _ *codecentraltest.Server) { h.Unreadable("acme/web") }, "can't read the repository acme/web", "owner/name"},
		{"no write access", func(h *ciwatchertest.Harness, _ *codecentraltest.Server) {
			h.Answer(fakegh.Answer{Args: []string{"api", repoPath}, Stdout: readOnly})
		}, "can't write to acme/web", "write access"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := ciwatchertest.New(t)
			h.Answer(fakegh.Answer{Args: []string{"api", repoPath}, Stdout: write})
			srv := checking(t, h, h.Bin)
			c.setup(h, srv)
			var e *core.Error
			err := turnOn(srv)
			if !errors.As(err, &e) || e.Code != core.Invalid || e.Field != "method" {
				t.Fatalf("err = %v, want Pull Request refused", err)
			}
			if !strings.Contains(e.Message, c.reason) || !strings.Contains(e.Fix, c.fix) {
				t.Fatalf("refusal %q, fix %q; want %q and %q", e.Message, e.Fix, c.reason, c.fix)
			}
			if got := method(t, srv); got == store.LandingPullRequest {
				t.Fatal("the method turned on although its check failed")
			}
		})
	}
}

func TestThePushRemotesOwnRefusalsComeBeforeTheCheck(t *testing.T) {
	h := ciwatchertest.New(t)
	srv := codecentraltest.NewServer(t, codecentraltest.WithPullRequestCheck(pullrequestwatcher.Checker(h.CodeCentral, h.Environ(h.Bin))))
	must(t, func() error {
		_, err := srv.Core.CreateProject(human, core.CreateProjectInput{Key: "acme"})
		return err
	})
	if err := turnOn(srv); err == nil || !strings.Contains(err.Error(), "Push remote") {
		t.Fatalf("err = %v, want the missing Push remote URL refused", err)
	}
	if n := len(h.Calls()); n != 0 {
		t.Fatalf("%d gh calls, want none before there is a Code Host", n)
	}
}
