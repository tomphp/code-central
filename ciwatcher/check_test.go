package ciwatcher_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/ciwatcher"
	"github.com/tomoram/code-central/ciwatcher/ciwatchertest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fakegh"
	"github.com/tomoram/code-central/store"
)

func TestMain(m *testing.M) { ciwatchertest.Main(m) }

// check runs the check mode for acme/web with the gh path Option ghPath,
// gh on path, and install places, the folders gh is otherwise looked for in.
func check(h *ciwatchertest.Harness, ghPath string, path []string, places ...string) core.CICheckResult {
	if places == nil {
		places = []string{}
	}
	in := core.CICheckInput{Provider: store.CIGitHubActions, Repository: "acme/web", GHPath: ghPath}
	return ciwatcher.Check(context.Background(), in, ciwatcher.Env{Environ: h.Environ(path...), InstallPlaces: places})
}

// ran is the programs the fake gh ran as, one per call.
func ran(h *ciwatchertest.Harness) []string {
	var out []string
	for _, c := range h.Calls() {
		out = append(out, c.Program)
	}
	return out
}

func TestTheCheckAsksGHForItsLoginThenTheRepository(t *testing.T) {
	h := ciwatchertest.New(t)
	if r := check(h, "", []string{h.Bin}); r != (core.CICheckResult{}) {
		t.Fatalf("check = %+v, want no problem", r)
	}
	var got [][]string
	for _, c := range h.Calls() {
		got = append(got, c.Args)
	}
	want := [][]string{{"auth", "status", "--hostname", "github.com"}, {"api", "repos/acme/web", "--jq", ".full_name"}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("gh calls = %q, want %q", got, want)
	}
}

func TestALoggedOutGHFailsTheCheckWithGHAuthLogin(t *testing.T) {
	h := ciwatchertest.New(t)
	h.LoggedOut()
	r := check(h, "", []string{h.Bin})
	if r.Problem != core.CILoggedOut || !strings.Contains(r.Reason, "You are not logged into any GitHub hosts") || !strings.Contains(r.Fix, "gh auth login") {
		t.Fatalf("check = %+v, want logged out with gh's message and gh auth login", r)
	}
	if n := len(h.Calls()); n != 1 {
		t.Fatalf("%d gh calls, want only gh auth status", n)
	}
}

func TestAnUnreadableRepositoryFailsTheCheckNamingIt(t *testing.T) {
	h := ciwatchertest.New(t)
	h.Unreadable("acme/web")
	r := check(h, "", []string{h.Bin})
	if r.Problem != core.CIRepositoryUnreadable || !strings.Contains(r.Reason, "acme/web") || !strings.Contains(r.Reason, "Not Found (HTTP 404)") ||
		!strings.Contains(r.Fix, "owner/name") {
		t.Fatalf("check = %+v, want the repository unreadable, naming it and gh's answer", r)
	}
}

func TestGHMissingEverywhereFailsTheCheckWithItsInstall(t *testing.T) {
	h := ciwatchertest.New(t)
	place := t.TempDir()
	r := check(h, "", []string{t.TempDir()}, place)
	if r.Problem != core.CIGHMissing || !strings.Contains(r.Reason, "PATH") || !strings.Contains(r.Reason, place) || !strings.Contains(r.Fix, "https://cli.github.com") ||
		!strings.Contains(r.Fix, "gh path Option") {
		t.Fatalf("check = %+v, want gh missing, naming where it looked, with how to install it", r)
	}
	if n := len(h.Calls()); n != 0 {
		t.Fatalf("%d gh calls with no gh", n)
	}
}

func TestGHIsFoundOnPATHBeforeTheInstallPlaces(t *testing.T) {
	h := ciwatchertest.New(t)
	place := h.GH(filepath.Join(t.TempDir(), "homebrew"))
	empty := t.TempDir()
	check(h, "", []string{empty, h.Bin}, filepath.Dir(place))
	if got := ran(h); len(got) != 2 || filepath.Dir(got[0]) != h.Bin {
		t.Fatalf("gh ran as %q, want the one on PATH in %s", got, h.Bin)
	}
}

func TestGHIsFoundInTheInstallPlacesWhenNotOnPATH(t *testing.T) {
	h := ciwatchertest.New(t)
	second := h.GH(filepath.Join(t.TempDir(), "linuxbrew"))
	if r := check(h, "", []string{t.TempDir()}, t.TempDir(), filepath.Dir(second)); r.Problem != "" {
		t.Fatalf("check = %+v, want gh found in an install place", r)
	}
	if got := ran(h); len(got) != 2 || got[0] != second {
		t.Fatalf("gh ran as %q, want %s", got, second)
	}
}

func TestTheGHPathOptionComesFirst(t *testing.T) {
	h := ciwatchertest.New(t)
	chosen := h.GH(filepath.Join(t.TempDir(), "tools"))
	for _, option := range []string{chosen, filepath.Dir(chosen)} {
		t.Run(option, func(t *testing.T) {
			before := len(h.Calls())
			if r := check(h, option, []string{h.Bin}); r.Problem != "" {
				t.Fatalf("check = %+v, want gh found through the Option", r)
			}
			if got := ran(h)[before:]; len(got) != 2 || got[0] != chosen {
				t.Fatalf("gh ran as %q, want %s, the Option's, not the one on PATH", got, chosen)
			}
		})
	}
}

func TestAGHPathOptionWithNoGHFailsTheCheckWithoutLookingElsewhere(t *testing.T) {
	h := ciwatchertest.New(t)
	option := filepath.Join(t.TempDir(), "gh")
	r := check(h, option, []string{h.Bin})
	if r.Problem != core.CIGHMissing || !strings.Contains(r.Reason, option) || !strings.Contains(r.Fix, "gh path Option") {
		t.Fatalf("check = %+v, want gh missing naming the Option's path", r)
	}
	if n := len(h.Calls()); n != 0 {
		t.Fatalf("the gh on PATH ran %d times despite the Option", n)
	}
}

func TestTheDefaultInstallPlaces(t *testing.T) {
	unix := []string{"/opt/homebrew/bin", "/usr/local/bin", "/home/linuxbrew/.linuxbrew/bin"}
	if got := ciwatcher.DefaultInstallPlaces(func(string) string { return "" }); !slices.Equal(got, unix) {
		t.Fatalf("install places = %q, want %q", got, unix)
	}
	pf := filepath.Join(t.TempDir(), "Program Files")
	got := ciwatcher.DefaultInstallPlaces(func(k string) string {
		if k == "ProgramFiles" {
			return pf
		}
		return ""
	})
	if want := append(unix, filepath.Join(pf, "GitHub CLI")); !slices.Equal(got, want) {
		t.Fatalf("install places = %q, want %q", got, want)
	}
}

func TestGHThatDoesntRunFailsTheCheck(t *testing.T) {
	h := ciwatchertest.New(t)
	h.Answer(fakegh.Answer{Args: []string{"auth"}, Exit: 0})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	in := core.CICheckInput{Provider: store.CIGitHubActions, Repository: "acme/web"}
	r := ciwatcher.Check(ctx, in, ciwatcher.Env{Environ: h.Environ(h.Bin), InstallPlaces: []string{}})
	if r.Problem != core.CICheckFailed || !strings.Contains(r.Reason, "didn't run") {
		t.Fatalf("check = %+v, want the check failed as gh didn't run", r)
	}
}
