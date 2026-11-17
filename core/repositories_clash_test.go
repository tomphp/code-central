package core_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
)

// The duplicate check on resolved clone paths (Spec code-central-879): a
// Repository's clone is at its local path, else beside the Home
// Repository's clone, which the core knows only from the Factory's latest
// Repositories report.

// reportedHome is a Factory online whose Repositories report holds the
// Home Repository's clone, at srcPath("acme"): the default path of a
// Repository is then srcPath(<its name>).
func (f fixture) reportedHome(t *testing.T) {
	t.Helper()
	beat(f, t, "cc", "box", core.HeartbeatOnline, 10)
	f.reportRepositories(t, core.RepositoryReport{Name: "acme", Path: srcPath("acme"), State: core.CloneReady})
}

// wantMessageNaming fails unless err's message names who.
func wantMessageNaming(t *testing.T, err error, who string) {
	t.Helper()
	var e *core.Error
	if !errors.As(err, &e) || !strings.Contains(e.Message, who) {
		t.Fatalf("error %v, want a message naming %s", err, who)
	}
}

func TestAddRepositoryRefusesAClashOfResolvedClonePathsWhileTheHomeCloneIsReported(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	f.addRepository(t, core.AddRepositoryInput{Name: "docs", LocalPath: srcPath("docs")})
	f.reportedHome(t)
	for name, tc := range map[string]struct {
		in    core.AddRepositoryInput
		other string
	}{
		"a local path at another's default path":          {core.AddRepositoryInput{Name: "api", LocalPath: srcPath("web")}, "web"},
		"a local path equal to the Home's own clone path": {core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git", LocalPath: srcPath("acme") + "/"}, "Home"},
		"a local path another Repository is at":           {core.AddRepositoryInput{Name: "api", LocalPath: srcPath("docs")}, "docs"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := f.core.AddRepository(human, "acme", tc.in)
			wantFix(t, err, core.Invalid, "local_path")
			wantMessageNaming(t, err, tc.other)
		})
	}
	if got := repositoryNames(f.projectView(t, "acme")); !slices.Equal(got, []string{"acme", "web", "docs"}) {
		t.Fatalf("repositories = %v after refusals, want acme, web, docs", got)
	}

	// The same clash from the other side: a Repository with no local path,
	// at the default path another has as its local path.
	g := newFixture(t)
	g.project(t, "acme")
	g.addRepository(t, core.AddRepositoryInput{Name: "api", LocalPath: srcPath("web")})
	g.reportedHome(t)
	_, err := g.core.AddRepository(human, "acme", core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	wantFix(t, err, core.Invalid, "local_path")
	wantMessageNaming(t, err, "api")
}

func TestEditRepositoryRefusesAClashOfResolvedClonePathsWhileTheHomeCloneIsReported(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git", LocalPath: srcPath("elsewhere")})
	f.addRepository(t, core.AddRepositoryInput{Name: "api", LocalPath: srcPath("api")})
	f.reportedHome(t)
	home, api, webDefault, none := srcPath("acme"), srcPath("api"), srcPath("web"), ""
	for name, in := range map[string]core.EditRepositoryInput{
		"a local path equal to the Home's clone path": {Name: "web", LocalPath: &home},
		"a local path another is at":                  {Name: "web", LocalPath: &api},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := f.core.EditRepository(human, "acme", in)
			wantFix(t, err, core.Invalid, "local_path")
		})
	}
	// web is elsewhere, so api can take web's default path, and then web
	// can't go back there.
	if _, err := f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "api", LocalPath: &webDefault}); err != nil {
		t.Fatalf("api to web's default path while web is elsewhere: %v", err)
	}
	_, err := f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "web", LocalPath: &none})
	wantFix(t, err, core.Invalid, "local_path")
	wantMessageNaming(t, err, "api")
}

func TestWithoutAHomeCloneReportOnlyExplicitLocalPathsAreComparedAsBefore(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	// Nothing to resolve default paths from, so this is accepted.
	f.addRepository(t, core.AddRepositoryInput{Name: "api", LocalPath: srcPath("web")})
	// Explicit local paths are compared as ever.
	_, err := f.core.AddRepository(human, "acme", core.AddRepositoryInput{Name: "dup", LocalPath: srcPath("web")})
	wantFix(t, err, core.Invalid, "local_path")
	// A report from a Factory since gone offline is no report.
	f.reportedHome(t)
	f.clock.Advance(time.Hour)
	f.addRepository(t, core.AddRepositoryInput{Name: "home-twin", LocalPath: srcPath("acme")})
}
