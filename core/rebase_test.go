package core_test

import (
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// Rebase (Spec code-central-1063): a Project-wide Merge and Pull Request
// option, stored beside Land per Spec and kept whatever the method.

func TestRebaseIsOffUntilSetAndLeftOutKeepsTheStoredValue(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	if f.landingOf(t).Rebase {
		t.Fatal("rebase on for a Project that never set it")
	}

	on, off := true, false
	set := func(in api.SetLandingMethodInput) api.LandingView {
		t.Helper()
		in.Remote = "origin"
		v, err := f.core.SetLandingMethod(human, "acme", in)
		if err != nil {
			t.Fatalf("set_landing_method %+v: %v", in, err)
		}
		return v.Landing
	}
	if v := set(api.SetLandingMethodInput{Method: domain.LandingMerge, Rebase: &on}); !v.Rebase {
		t.Fatal("rebase off after setting it on")
	}
	if v := set(api.SetLandingMethodInput{Method: domain.LandingMerge, Push: true}); !v.Rebase {
		t.Fatal("rebase off after a set_landing_method that left it out")
	}
	// Kept whatever the method: Branch keeps it, though not in force.
	if v := set(api.SetLandingMethodInput{Method: domain.LandingBranch}); !v.Rebase {
		t.Fatal("rebase off after switching to Branch")
	}
	if !f.landingOf(t).Rebase {
		t.Fatal("the landing view doesn't show rebase on")
	}
	if v := set(api.SetLandingMethodInput{Method: domain.LandingMerge, Rebase: &off}); v.Rebase {
		t.Fatal("rebase on after setting it off")
	}
	// Land per Spec and Rebase are independent.
	if v := set(api.SetLandingMethodInput{Method: domain.LandingMerge, Rebase: &on, LandPerSpec: &off}); !v.Rebase || v.LandPerSpec {
		t.Fatalf("rebase %v, land per spec %v, want on and off", v.Rebase, v.LandPerSpec)
	}
}

func TestRebaseIsInForceUnderMergeAndPullRequestButNotBranch(t *testing.T) {
	f, _ := prWorld(t)
	on := true
	for _, c := range []struct {
		method domain.LandingMethod
		want   bool
	}{
		{domain.LandingMerge, true},
		{domain.LandingBranch, false},
		{domain.LandingPullRequest, true},
	} {
		v, err := f.core.SetLandingMethod(human, "acme", api.SetLandingMethodInput{Method: c.method, Remote: "origin", Rebase: &on})
		if err != nil {
			t.Fatalf("set_landing_method %s: %v", c.method, err)
		}
		ps := v.LandingPolicies()
		if got := ps.RebaseInForce(); got != c.want {
			t.Errorf("RebaseInForce under %s = %v, want %v", c.method, got, c.want)
		}
		if got := ps.In("acme").RebaseInForce(); got != c.want {
			t.Errorf("acme's RebaseInForce under %s = %v, want %v", c.method, got, c.want)
		}
	}
}
