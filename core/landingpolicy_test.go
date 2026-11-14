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
