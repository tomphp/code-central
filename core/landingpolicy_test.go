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
						"LandsByPullRequest":           {NewLandingPolicy(m, push, lps, ci).LandsByPullRequest(), pr},
						"RunsPullRequestWatcher":       {NewLandingPolicy(m, push, lps, ci).RunsPullRequestWatcher(), pr},
						"RunsCIWatcher":                {NewLandingPolicy(m, push, lps, ci).RunsCIWatcher(), ci},
						"MergesSpecBranches":           {NewLandingPolicy(m, push, lps, ci).MergesSpecBranches(), merge && lps},
						"PushesSpecBranches":           {NewLandingPolicy(m, push, lps, ci).PushesSpecBranches(), pr && lps},
						"MergesLeftoverSpecBranches":   {NewLandingPolicy(m, push, lps, ci).MergesLeftoverSpecBranches(), merge},
						"PushesAcceptedBranch":         {NewLandingPolicy(m, push, lps, ci).PushesAcceptedBranch(), branch && effPush},
						"KeepsAcceptedWorktree":        {NewLandingPolicy(m, push, lps, ci).KeepsAcceptedWorktree(), !branch},
						"EveryRepositoryNeedsCloneURL": {NewLandingPolicy(m, push, lps, ci).EveryRepositoryNeedsCloneURL(), pr},
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

// TestARepositorysLandingPolicy checks that a local-only Repository reads
// the Project's policy with Push off, so Merge + Push reads as Merge and
// Branch + Push as Branch there, and that every other Repository, the
// Home among them, reads it unchanged.
func TestARepositorysLandingPolicy(t *testing.T) {
	repositories := map[string]struct {
		r         RepositoryView
		localOnly bool
	}{
		"the Home":                      {RepositoryView{Name: "acme", Home: true, CloneURL: "git@github.com:acme/acme.git"}, false},
		"the Home before its report":    {RepositoryView{Name: "acme", Home: true}, false},
		"a Repository with a Clone URL": {RepositoryView{Name: "web", CloneURL: "git@github.com:acme/web.git"}, false},
		"a local-only Repository":       {RepositoryView{Name: "notes", LocalPath: "/src/notes"}, true},
	}
	for _, m := range []store.LandingMethod{store.LandingMerge, store.LandingBranch, store.LandingPullRequest} {
		for _, push := range []bool{false, true} {
			for _, lps := range []bool{false, true} {
				for _, ci := range []bool{false, true} {
					for name, c := range repositories {
						got := NewLandingPolicy(m, push, lps, ci).ForRepository(c.r)
						if want := NewLandingPolicy(m, push && !c.localOnly, lps, ci); got != want {
							t.Errorf("%s's policy under (method %s, push %v, land per spec %v, ci %v) = %+v, want %+v", name, m, push, lps, ci, got, want)
						}
					}
				}
			}
		}
	}
	local := repositories["a local-only Repository"].r
	if lp := NewLandingPolicy(store.LandingMerge, true, false, true).ForRepository(local); lp.PushesBase() || lp.KeepsBaseLevel() || lp.CIGateHolds() || lp.OffersLandAnyway() || lp.RecordsPushedCommit() {
		t.Errorf("Merge + Push in a local-only Repository = %+v, want Merge's: no push, no fetch-and-level, no CI gate, no Land anyway and no pushed commit", lp)
	}
	if lp := NewLandingPolicy(store.LandingBranch, true, false, false).ForRepository(local); lp.PushesAcceptedBranch() {
		t.Errorf("Branch + Push in a local-only Repository = %+v, want Branch's: no branch push on accept", lp)
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
