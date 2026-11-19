package domain

import "testing"

// TestLandingPolicyMatrix pins every rule over every Method × Push × Land
// per Spec × Rebase × CI Tracking combination.
func TestLandingPolicyMatrix(t *testing.T) {
	for _, m := range []LandingMethod{LandingMerge, LandingBranch, LandingPullRequest} {
		for _, push := range []bool{false, true} {
			for _, lps := range []bool{false, true} {
				for _, rebase := range []bool{false, true} {
					for _, ci := range []bool{false, true} {
						pr, merge, branch := m == LandingPullRequest, m == LandingMerge, m == LandingBranch
						// Pull Request with Push on reads as Push off.
						effPush := push && !pr
						lp := NewLandingPolicy(m, push, lps, rebase, ci)
						want := map[string][2]bool{
							"LandPerSpecInForce":  {lp.LandPerSpecInForce(), (merge || pr) && lps},
							"CIGateHolds":         {lp.CIGateHolds(), merge && effPush && ci},
							"RecordsPushedCommit": {lp.RecordsPushedCommit(), pr || (effPush && ci)},
							"TracksCIResults":     {lp.TracksCIResults(), !(pr && !ci)},
							"NeedsPushRemote":     {lp.NeedsPushRemote(), effPush || pr},
							"PushesBase":          {lp.PushesBase(), merge && effPush},
							"KeepsBaseLevel":      {lp.KeepsBaseLevel(), pr || (merge && effPush)},
							"LandsOnHumanMerge":   {lp.LandsOnHumanMerge(), branch || pr},
							// Today's code offers it under Merge without Push too;
							// the glossary's false holds there.
							"OffersLandAnyway":           {lp.OffersLandAnyway(), merge && effPush},
							"FlagsNotLanded":             {lp.FlagsNotLanded(), merge},
							"LandsByPullRequest":         {lp.LandsByPullRequest(), pr},
							"RunsCIWatcher":              {lp.RunsCIWatcher(), ci},
							"MergesSpecBranches":         {lp.MergesSpecBranches(), merge && lps},
							"PushesSpecBranches":         {lp.PushesSpecBranches(), pr && lps},
							"MergesLeftoverSpecBranches": {lp.MergesLeftoverSpecBranches(), merge},
							"PushesAcceptedBranch":       {lp.PushesAcceptedBranch(), branch && effPush},
							"KeepsAcceptedWorktree":      {lp.KeepsAcceptedWorktree(), !branch},
							"NeedsCloneURL":              {lp.NeedsCloneURL(), pr},
							"OffersPush":                 {lp.OffersPush(), merge || branch},
							"OffersLandPerSpec":          {lp.OffersLandPerSpec(), merge || pr},
							"OffersRebase":               {lp.OffersRebase(), merge || pr},
							"RebaseInForce":              {lp.RebaseInForce(), (merge || pr) && rebase},
						}
						for name, v := range want {
							if v[0] != v[1] {
								t.Errorf("%s(method %s, push %v, land per spec %v, rebase %v, ci %v) = %v, want %v", name, m, push, lps, rebase, ci, v[0], v[1])
							}
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
							got := NewLandingPolicy(m, push, lps, false, ci).Route(onSpecBranch, prMerged)
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
