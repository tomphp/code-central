package ciwatcher_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/ciwatcher"
	"github.com/tomoram/code-central/ciwatcher/ciwatchertest"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

var ctx = context.Background()

// serviceWith is a real Code Central with a Project acme whose CI check
// runs the test binary as code-central, as the service runs its own
// executable, with gh found on path. Its client is the human's.
func serviceWith(t *testing.T, h *ciwatchertest.Harness, path ...string) *codecentraltest.Server {
	t.Helper()
	srv := codecentraltest.NewServer(t, codecentraltest.WithCICheck(ciwatcher.Checker(h.CodeCentral, h.Environ(path...))))
	if _, err := srv.Client.CreateProject(ctx, core.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	srv.Client.Actor = core.Human
	return srv
}

// setGHPath sets the gh path Option, as the human.
func setGHPath(t *testing.T, srv *codecentraltest.Server, gh string) {
	t.Helper()
	if _, err := srv.Client.SetOptions(ctx, core.SetOptionsInput{GHPath: &gh}); err != nil {
		t.Fatal(err)
	}
}

func turnOn(srv *codecentraltest.Server) error {
	_, err := srv.Client.SetCITracking(ctx, "acme", core.SetCITrackingInput{Tracking: true, Repository: "acme/web"})
	return err
}

// refusal is err as the refusal of turning CI Tracking on.
func refusal(t *testing.T, err error) *core.Error {
	t.Helper()
	var e *core.Error
	if !errors.As(err, &e) || e.Code != core.Invalid || e.Field != "tracking" {
		t.Fatalf("err = %v, want CI Tracking refused", err)
	}
	return e
}

// tracking is whether acme has CI Tracking on, as the HTTP API reads it.
func tracking(t *testing.T, srv *codecentraltest.Server) bool {
	t.Helper()
	p, err := srv.Client.GetProject(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	return p.CI.Tracking
}

func TestCITrackingTurnsOnWhenTheCheckPasses(t *testing.T) {
	h := ciwatchertest.New(t)
	srv := serviceWith(t, h, h.Bin)
	if err := turnOn(srv); err != nil || !tracking(t, srv) {
		t.Fatalf("turning on: %v; tracking %v", err, tracking(t, srv))
	}
	if n := len(h.Calls()); n != 2 {
		t.Fatalf("%d gh calls, want auth status and the repository", n)
	}
}

func TestEachFailedCheckRefusesCITrackingWithItsFix(t *testing.T) {
	// gh missing is a gh path Option with no gh there: with none, the
	// check would look in the real install places, where this machine
	// may have gh. The in-process tests cover looking there.
	noGH := filepath.Join(t.TempDir(), "gh")
	for _, c := range []struct {
		name        string
		setup       func(h *ciwatchertest.Harness, srv *codecentraltest.Server)
		reason, fix string
	}{
		{"gh missing", func(_ *ciwatchertest.Harness, srv *codecentraltest.Server) { setGHPath(t, srv, noGH) }, "gh isn't there", "gh path Option"},
		{"logged out", func(h *ciwatchertest.Harness, _ *codecentraltest.Server) { h.LoggedOut() }, "isn't logged in", "gh auth login"},
		{"repository unreadable", func(h *ciwatchertest.Harness, _ *codecentraltest.Server) { h.Unreadable("acme/web") }, "can't read the repository acme/web", "owner/name"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := ciwatchertest.New(t)
			srv := serviceWith(t, h, h.Bin)
			c.setup(h, srv)
			e := refusal(t, turnOn(srv))
			if !strings.Contains(e.Message, c.reason) || !strings.Contains(e.Fix, c.fix) {
				t.Fatalf("refusal %q, fix %q; want %q and %q", e.Message, e.Fix, c.reason, c.fix)
			}
			if tracking(t, srv) {
				t.Fatal("a failed check turned CI Tracking on")
			}
		})
	}
}

func TestTheCheckFindsGHThroughTheGHPathOption(t *testing.T) {
	h := ciwatchertest.New(t)
	srv := serviceWith(t, h, t.TempDir())
	gh := h.GH(filepath.Join(t.TempDir(), "tools"))
	setGHPath(t, srv, gh)
	if err := turnOn(srv); err != nil {
		t.Fatalf("turning on with gh through the gh path Option: %v", err)
	}
	if calls := h.Calls(); len(calls) != 2 || calls[0].Program != gh {
		t.Fatalf("gh calls = %+v, want two from %s", calls, gh)
	}
}

func TestTurningCITrackingOffRunsNoCheck(t *testing.T) {
	h := ciwatchertest.New(t)
	srv := serviceWith(t, h, h.Bin)
	if err := turnOn(srv); err != nil {
		t.Fatal(err)
	}
	h.LoggedOut()
	before := len(h.Calls())
	if _, err := srv.Client.SetCITracking(ctx, "acme", core.SetCITrackingInput{Repository: "acme/web"}); err != nil || tracking(t, srv) {
		t.Fatalf("turning off: %v", err)
	}
	if n := len(h.Calls()); n != before {
		t.Fatalf("turning off ran gh %d times", n-before)
	}
}

func TestACheckThatDoesntFinishRefusesCITracking(t *testing.T) {
	h := ciwatchertest.New(t)
	srv := codecentraltest.NewServer(t, codecentraltest.WithCICheck(ciwatcher.Checker(filepath.Join(t.TempDir(), "missing"), h.Environ(h.Bin))))
	if _, err := srv.Client.CreateProject(ctx, core.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	srv.Client.Actor = core.Human
	if e := refusal(t, turnOn(srv)); !strings.Contains(e.Message, "didn't finish") || e.Fix == "" {
		t.Fatalf("refusal %+v, want the check that didn't finish, with a fix", e)
	}
}
