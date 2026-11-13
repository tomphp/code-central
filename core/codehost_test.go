package core_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Pull Request as a Landing Method (code-central-387): the Code Host
// comes from the Push remote URL the Factory reported.

func (f fixture) pushRemote(t *testing.T, url string) {
	t.Helper()
	if _, err := f.core.ReportPushRemote(human, "acme", core.ReportPushRemoteInput{URL: url}); err != nil {
		t.Fatalf("report_push_remote: %v", err)
	}
}

func (f fixture) landingOf(t *testing.T) core.LandingView {
	t.Helper()
	ps, err := f.core.ListProjects(human)
	if err != nil {
		t.Fatal(err)
	}
	return ps[0].Landing
}

func TestPullRequestIsRefusedWithAFixWhenNoPushRemoteURLWasReported(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	_, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest})
	e, ok := err.(*core.Error)
	if !ok || e.Code != core.Invalid || e.Field != "method" || e.Fix == "" {
		t.Fatalf("err = %#v, want Invalid on method with a fix", err)
	}
	if got := f.landingOf(t).Method; got != store.LandingMerge {
		t.Fatalf("method = %s, want merge kept", got)
	}
}

func TestPullRequestIsRefusedForAnUnsupportedHost(t *testing.T) {
	for _, url := range []string{"git@gitlab.com:acme/web.git", "https://github.example.com/acme/web.git", "https://dev.azure.com/acme/web"} {
		f := newFixture(t)
		f.project(t, "acme")
		f.pushRemote(t, url)
		_, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest})
		if e, ok := err.(*core.Error); !ok || e.Code != core.Invalid || e.Fix == "" {
			t.Errorf("%s: err = %#v, want Invalid with a fix", url, err)
		}
	}
}

func TestPullRequestWorksOutTheCodeHostFromTheReportedURL(t *testing.T) {
	for _, c := range []struct{ url, host, repo string }{
		{"git@github.com:acme/web.git", "github", "acme/web"},
		{"https://tok@github.com/acme/web", "github", "acme/web"},
		{"https://dev.azure.com/acme/shop/_git/web", "azure_repos", "acme/shop/web"},
		{"https://acme@dev.azure.com/acme/shop/_git/web", "azure_repos", "acme/shop/web"},
		{"git@ssh.dev.azure.com:v3/acme/shop/web", "azure_repos", "acme/shop/web"},
		{"https://acme.visualstudio.com/shop/_git/web", "azure_repos", "acme/shop/web"},
		{"https://acme.visualstudio.com/DefaultCollection/shop/_git/web", "azure_repos", "acme/shop/web"},
		{"acme@vs-ssh.visualstudio.com:v3/acme/shop/web", "azure_repos", "acme/shop/web"},
	} {
		f := newFixture(t)
		f.project(t, "acme")
		f.pushRemote(t, c.url)
		v, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest, Push: false, Remote: "origin"})
		if err != nil {
			t.Errorf("%s: %v", c.url, err)
			continue
		}
		if v.Landing.Method != store.LandingPullRequest || v.Landing.CodeHost != c.host || v.Landing.Repository != c.repo {
			t.Errorf("%s: landing = %+v, want %s %s", c.url, v.Landing, c.host, c.repo)
		}
		if got := f.landingOf(t); got.CodeHost != c.host || got.Repository != c.repo {
			t.Errorf("%s: read back %+v", c.url, got)
		}
	}
}

func TestThePATVariableDefaultsAndIsKeptAndValidated(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.pushRemote(t, "https://dev.azure.com/acme/shop/_git/web")
	if got := f.landingOf(t).PATEnv; got != "ADO_PAT" {
		t.Fatalf("default pat_env = %q", got)
	}
	v, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest, PATEnv: " REPOS_PAT "})
	if err != nil || v.Landing.PATEnv != "REPOS_PAT" {
		t.Fatalf("set = %+v, %v", v.Landing, err)
	}
	v, err = f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest})
	if err != nil || v.Landing.PATEnv != "REPOS_PAT" {
		t.Fatalf("left out should keep: %+v, %v", v.Landing, err)
	}
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest, PATEnv: "not a name"}); err == nil {
		t.Fatal("a bad variable name was accepted")
	}
}

func TestPullRequestIgnoresTheStoredPushFlag(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.pushRemote(t, "git@github.com:acme/web.git")
	v, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest, Push: true})
	if err != nil || v.Landing.Push {
		t.Fatalf("landing = %+v, %v; want Push not stored", v.Landing, err)
	}
}

func TestPullRequestNeedsNoCheckToTurnOff(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.pushRemote(t, "git@github.com:acme/web.git")
	f.setLanding(t, store.LandingPullRequest)
	f.setLanding(t, store.LandingBranch)
	if got := f.landingOf(t).Method; got != store.LandingBranch {
		t.Fatalf("method = %s", got)
	}
}
