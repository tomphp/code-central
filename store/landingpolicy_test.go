package store

import "testing"

func TestNormalizedClearsPushUnderPullRequestOnly(t *testing.T) {
	if (Landing{Method: LandingPullRequest, Push: true}).Normalized().Push {
		t.Error("Pull Request kept Push")
	}
	if !(Landing{Method: LandingMerge, Push: true}).Normalized().Push {
		t.Error("Merge lost Push")
	}
}
