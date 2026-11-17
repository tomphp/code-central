package core

import (
	"testing"

	"github.com/tomoram/code-central/store"
)

func TestLandingPolicyMatrix(t *testing.T) {
	for _, m := range []store.LandingMethod{store.LandingMerge, store.LandingBranch, store.LandingPullRequest} {
		for _, push := range []bool{false, true} {
			for _, lps := range []bool{false, true} {
				for _, ci := range []bool{false, true} {
					pr, merge, branch := m == store.LandingPullRequest, m == store.LandingMerge, m == store.LandingBranch
					// Pull Request with Push on reads as Push off.
					effPush := push && !pr
					want := map[string][2]bool{
						"LandPerSpecInForce":  {NewLandingPolicy(m, push, lps, ci).LandPerSpecInForce(), (merge || pr) && lps},
						"CIGateHolds":         {NewLandingPolicy(m, push, lps, ci).CIGateHolds(), merge && effPush && ci},
						"RecordsPushedCommit": {NewLandingPolicy(m, push, lps, ci).RecordsPushedCommit(), pr || (effPush && ci)},
						"TracksCIResults":     {NewLandingPolicy(m, push, lps, ci).TracksCIResults(), !(pr && !ci)},
						"NeedsPushRemote":     {NewLandingPolicy(m, push, lps, ci).NeedsPushRemote(), effPush || pr},
						"PushesBase":          {NewLandingPolicy(m, push, lps, ci).PushesBase(), merge && effPush},
						"KeepsBaseLevel":      {NewLandingPolicy(m, push, lps, ci).KeepsBaseLevel(), pr || (merge && effPush)},
						"LandsOnHumanMerge":   {NewLandingPolicy(m, push, lps, ci).LandsOnHumanMerge(), branch || pr},
						// Today's code offers it under Merge without Push too;
						// the glossary's false holds there.
						"OffersLandAnyway":             {NewLandingPolicy(m, push, lps, ci).OffersLandAnyway(), merge && effPush},
						"FlagsNotLanded":               {NewLandingPolicy(m, push, lps, ci).FlagsNotLanded(), merge},
						"LandsByPullRequest":         {NewLandingPolicy(m, push, lps, ci).LandsByPullRequest(), pr},
						"RunsCIWatcher":              {NewLandingPolicy(m, push, lps, ci).RunsCIWatcher(), ci},
						"MergesSpecBranches":         {NewLandingPolicy(m, push, lps, ci).MergesSpecBranches(), merge && lps},
						"PushesSpecBranches":         {NewLandingPolicy(m, push, lps, ci).PushesSpecBranches(), pr && lps},
						"MergesLeftoverSpecBranches": {NewLandingPolicy(m, push, lps, ci).MergesLeftoverSpecBranches(), merge},
						"PushesAcceptedBranch":       {NewLandingPolicy(m, push, lps, ci).PushesAcceptedBranch(), branch && effPush},
						"KeepsAcceptedWorktree":      {NewLandingPolicy(m, push, lps, ci).KeepsAcceptedWorktree(), !branch},
						"NeedsCloneURL":              {NewLandingPolicy(m, push, lps, ci).NeedsCloneURL(), pr},
					}
					for name, v := range want {
						if v[0] != v[1] {
							t.Errorf("%s(method %s, push %v, land per spec %v, ci %v) = %v, want %v", name, m, push, lps, ci, v[0], v[1])
						}
					}
				}
			}
		}
	}
}

// TestARepositorysLandingPolicy checks that a Repository's policy is its
// Own Landing, else the Default Landing, with Push off when it is
// local-only, so Merge + Push reads as Merge and Branch + Push as Branch
// there; that every other Repository, the Home among them, reads it
// unchanged; and that the Project-wide rules ask across the Repositories.
func TestARepositorysLandingPolicy(t *testing.T) {
	owns := []store.OwnLanding{{}, {Method: store.LandingMerge, Push: true}, {Method: store.LandingBranch}, {Method: store.LandingPullRequest}}
	for _, m := range []store.LandingMethod{store.LandingMerge, store.LandingBranch, store.LandingPullRequest} {
		for _, push := range []bool{false, true} {
			for _, lps := range []bool{false, true} {
				for _, ci := range []bool{false, true} {
					for _, own := range owns {
						p := store.Project{Key: "acme", Landing: store.Landing{Method: m, Push: push, LandPerSpec: lps}, CI: store.CI{Tracking: ci, PushRemoteURL: "git@github.com:acme/acme.git"}}
						p.SetRepositories([]store.Repository{
							{Name: "acme", Home: true, OwnLanding: own},
							{Name: "web", CloneURL: "git@github.com:acme/web.git", Base: "main", OwnLanding: own},
							{Name: "notes", LocalPath: "/src/notes", Base: "main", OwnLanding: own},
						})
						em, ep := m, push
						if own.Set() {
							em, ep = own.Method, own.Push
						}
						ps := landingPolicies(p)
						for name, localOnly := range map[string]bool{"acme": false, "web": false, "notes": true} {
							if got, want := ps.In(name), NewLandingPolicy(em, ep && !localOnly, lps, ci); got != want {
								t.Errorf("%s's policy under (default %s, push %v, own %+v, land per spec %v, ci %v) = %+v, want %+v", name, m, push, own, lps, ci, got, want)
							}
						}
						if got, want := ps.RunsPullRequestWatcher(), em == store.LandingPullRequest; got != want {
							t.Errorf("RunsPullRequestWatcher under (default %s, own %+v) = %v, want %v", m, own, got, want)
						}
						if got := ps.RunsCIWatcher(); got != ci {
							t.Errorf("RunsCIWatcher with CI Tracking %v = %v", ci, got)
						}
					}
				}
			}
		}
	}
	p := store.Project{Key: "acme", Landing: store.Landing{Method: store.LandingMerge, Push: true}, CI: store.CI{Tracking: true}}
	p.SetRepositories([]store.Repository{{Name: "acme", Home: true}, {Name: "notes", LocalPath: "/src/notes", Base: "main"}})
	if lp := landingPolicyIn(p, "notes"); lp.PushesBase() || lp.KeepsBaseLevel() || lp.CIGateHolds() || lp.OffersLandAnyway() || lp.RecordsPushedCommit() {
		t.Errorf("Merge + Push in a local-only Repository = %+v, want Merge's: no push, no fetch-and-level, no CI gate, no Land anyway and no pushed commit", lp)
	}
	p.Landing.Method = store.LandingBranch
	if lp := landingPolicyIn(p, "notes"); lp.PushesAcceptedBranch() {
		t.Errorf("Branch + Push in a local-only Repository = %+v, want Branch's: no branch push on accept", lp)
	}
	p.Landing.Method = store.LandingMerge
	p.SetRepositories([]store.Repository{{Name: "acme", Home: true}, {Name: "web", CloneURL: "git@github.com:acme/web.git", Base: "main", OwnLanding: store.OwnLanding{Method: store.LandingPullRequest}}})
	if ps := landingPolicies(p); !ps.RunsPullRequestWatcher() || ps.In("acme").LandsByPullRequest() {
		t.Errorf("one Repository under Pull Request: watcher %v, the Home by Pull Request %v; want the watcher on and the Home on Merge", ps.RunsPullRequestWatcher(), ps.In("acme").LandsByPullRequest())
	}
}

// TestLandingPolicyRoute checks Route against the if/else chain the
// Factory's land takes today, where onSpecBranch is what waitsOnSpecBranch
// reports and prMerged is whether the Ticket's pull request is merged.
func TestLandingPolicyRoute(t *testing.T) {
	today := func(m store.LandingMethod, onSpecBranch, prMerged bool) LandingRoute {
		if m == store.LandingBranch {
			return RouteBranch
		} else if m == store.LandingPullRequest && (prMerged || !onSpecBranch) {
			return RoutePullRequest
		} else if onSpecBranch {
			return RouteSpecBranch
		}
		return RouteMerge
	}
	for _, m := range []store.LandingMethod{store.LandingMerge, store.LandingBranch, store.LandingPullRequest} {
		for _, push := range []bool{false, true} {
			for _, lps := range []bool{false, true} {
				for _, ci := range []bool{false, true} {
					for _, onSpecBranch := range []bool{false, true} {
						for _, prMerged := range []bool{false, true} {
							got := NewLandingPolicy(m, push, lps, ci).Route(onSpecBranch, prMerged)
							if want := today(m, onSpecBranch, prMerged); got != want {
								t.Errorf("Route(%v, %v) under (method %s, push %v, land per spec %v, ci %v) = %v, want %v", onSpecBranch, prMerged, m, push, lps, ci, got, want)
							}
						}
					}
				}
			}
		}
	}
}
