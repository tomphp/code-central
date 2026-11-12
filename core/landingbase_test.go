package core_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// The base is a Project setting (code-central-419): set_landing_method
// stores it, the Project read resolves it.

func strp(s string) *string { return &s }

func (f fixture) landingBase(t *testing.T) string {
	t.Helper()
	v, err := f.core.GetProject(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	return v.Landing.Base
}

func TestTheBaseReadsMainWhenNothingIsStoredOrReported(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	if got := f.landingBase(t); got != "main" {
		t.Fatalf("base = %q, want main", got)
	}
}

func TestSetLandingMethodStoresTheBaseAndKeepsItWhenLeftOut(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingMerge, Base: strp("release/1.0")}); err != nil {
		t.Fatal(err)
	}
	if got := f.landingBase(t); got != "release/1.0" {
		t.Fatalf("base = %q, want release/1.0", got)
	}
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingBranch}); err != nil {
		t.Fatal(err)
	}
	if got := f.landingBase(t); got != "release/1.0" {
		t.Fatalf("base after leaving it out = %q, want release/1.0", got)
	}
}

func TestSetLandingMethodRejectsAnInvalidBase(t *testing.T) {
	for _, bad := range []string{"a b", "-x", "a..b", "x.lock", "a/.b", "a//b", "/a", "a/", "a.", "@", "a@{b", "a~b", "a^b", "a:b", "a?b", "a*b", "a[b", "a\\b"} {
		f := newFixture(t)
		f.project(t, "acme")
		_, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingMerge, Base: strp(bad)})
		if err == nil {
			t.Errorf("base %q was accepted", bad)
			continue
		}
		wantCode(t, err, core.Invalid, "base")
		if got := f.landingBase(t); got != "main" {
			t.Errorf("base %q was stored: %q", bad, got)
		}
	}
}

func TestTheBaseFallsBackToTheFrozenPushRemoteBaseAndReportPushRemoteLeavesItAlone(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	err := f.store.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.CI.PushRemoteBase = "trunk"
		return w.PutProject(p)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.landingBase(t); got != "trunk" {
		t.Fatalf("base = %q, want the stored PushRemoteBase trunk", got)
	}
	if _, err := f.core.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: "git@github.com:acme/web.git", Base: "develop"}); err != nil {
		t.Fatal(err)
	}
	if got := f.landingBase(t); got != "trunk" {
		t.Fatalf("report_push_remote changed the base to %q", got)
	}
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingMerge, Base: strp("release")}); err != nil {
		t.Fatal(err)
	}
	if got := f.landingBase(t); got != "release" {
		t.Fatalf("base = %q, want the stored release", got)
	}
}
