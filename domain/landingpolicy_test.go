package domain

import "testing"

// TestLandingPolicyMatrix pins every rule over every Method × Push × Land
// per Spec × CI Tracking combination.
func TestLandingPolicyMatrix(t *testing.T) {
	for _, m := range []LandingMethod{LandingMerge, LandingBranch, LandingPullRequest} {
		for _, push := range []bool{false, true} {
			for _, lps := range []bool{false, true} {
				for _, ci := range []bool{false, true} {
					pr, merge, branch := m == LandingPullRequest, m == LandingMerge, m == LandingBranch
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
						"OffersLandAnyway":           {NewLandingPolicy(m, push, lps, ci).OffersLandAnyway(), merge && effPush},
						"FlagsNotLanded":             {NewLandingPolicy(m, push, lps, ci).FlagsNotLanded(), merge},
						"LandsByPullRequest":         {NewLandingPolicy(m, push, lps, ci).LandsByPullRequest(), pr},
						"RunsCIWatcher":              {NewLandingPolicy(m, push, lps, ci).RunsCIWatcher(), ci},
						"MergesSpecBranches":         {NewLandingPolicy(m, push, lps, ci).MergesSpecBranches(), merge && lps},
						"PushesSpecBranches":         {NewLandingPolicy(m, push, lps, ci).PushesSpecBranches(), pr && lps},
						"MergesLeftoverSpecBranches": {NewLandingPolicy(m, push, lps, ci).MergesLeftoverSpecBranches(), merge},
						"PushesAcceptedBranch":       {NewLandingPolicy(m, push, lps, ci).PushesAcceptedBranch(), branch && effPush},
						"KeepsAcceptedWorktree":      {NewLandingPolicy(m, push, lps, ci).KeepsAcceptedWorktree(), !branch},
						"NeedsCloneURL":              {NewLandingPolicy(m, push, lps, ci).NeedsCloneURL(), pr},
						"OffersPush":                 {NewLandingPolicy(m, push, lps, ci).OffersPush(), merge || branch},
						"OffersLandPerSpec":          {NewLandingPolicy(m, push, lps, ci).OffersLandPerSpec(), merge || pr},
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

// TestLandingPolicyRoute checks Route against the if/else chain the
// Factory's land takes today, where onSpecBranch is what waitsOnSpecBranch
// reports and prMerged is whether the Ticket's pull request is merged.
func TestLandingPolicyRoute(t *testing.T) {
	today := func(m LandingMethod, onSpecBranch, prMerged bool) LandingRoute {
		if m == LandingBranch {
			return RouteBranch
		} else if m == LandingPullRequest && (prMerged || !onSpecBranch) {
			return RoutePullRequest
		} else if onSpecBranch {
			return RouteSpecBranch
		}
		return RouteMerge
	}
	for _, m := range []LandingMethod{LandingMerge, LandingBranch, LandingPullRequest} {
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
