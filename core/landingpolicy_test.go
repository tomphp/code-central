package core

import (
	"testing"

	"github.com/tomoram/code-central/domain"
)

// TestARepositorysLandingPolicy checks that a Repository's policy is its
// Own Landing, else the Default Landing, with Push off when it is
// local-only, so Merge + Push reads as Merge and Branch + Push as Branch
// there; that every other Repository, the Home among them, reads it
// unchanged; and that the Project-wide rules ask across the Repositories.
func TestARepositorysLandingPolicy(t *testing.T) {
	owns := []domain.OwnLanding{{}, {Method: domain.LandingMerge, Push: true}, {Method: domain.LandingBranch}, {Method: domain.LandingPullRequest}}
	for _, m := range []domain.LandingMethod{domain.LandingMerge, domain.LandingBranch, domain.LandingPullRequest} {
		for _, push := range []bool{false, true} {
			for _, lps := range []bool{false, true} {
				for _, ci := range []bool{false, true} {
					for _, own := range owns {
						p := domain.Project{Key: "acme", Landing: domain.Landing{Method: m, Push: push, LandPerSpec: lps}, CI: domain.CI{Tracking: ci, PushRemoteURL: "git@github.com:acme/acme.git"}}
						p.SetRepositories([]domain.Repository{
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
							if got, want := ps.In(name), domain.NewLandingPolicy(em, ep && !localOnly, lps, ci); got != want {
								t.Errorf("%s's policy under (default %s, push %v, own %+v, land per spec %v, ci %v) = %+v, want %+v", name, m, push, own, lps, ci, got, want)
							}
						}
						if got, want := ps.RunsPullRequestWatcher(), em == domain.LandingPullRequest; got != want {
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
	p := domain.Project{Key: "acme", Landing: domain.Landing{Method: domain.LandingMerge, Push: true}, CI: domain.CI{Tracking: true}}
	p.SetRepositories([]domain.Repository{{Name: "acme", Home: true}, {Name: "notes", LocalPath: "/src/notes", Base: "main"}})
	if lp := landingPolicyIn(p, "notes"); lp.PushesBase() || lp.KeepsBaseLevel() || lp.CIGateHolds() || lp.OffersLandAnyway() || lp.RecordsPushedCommit() {
		t.Errorf("Merge + Push in a local-only Repository = %+v, want Merge's: no push, no fetch-and-level, no CI gate, no Land anyway and no pushed commit", lp)
	}
	p.Landing.Method = domain.LandingBranch
	if lp := landingPolicyIn(p, "notes"); lp.PushesAcceptedBranch() {
		t.Errorf("Branch + Push in a local-only Repository = %+v, want Branch's: no branch push on accept", lp)
	}
	p.Landing.Method = domain.LandingMerge
	p.SetRepositories([]domain.Repository{{Name: "acme", Home: true}, {Name: "web", CloneURL: "git@github.com:acme/web.git", Base: "main", OwnLanding: domain.OwnLanding{Method: domain.LandingPullRequest}}})
	if ps := landingPolicies(p); !ps.RunsPullRequestWatcher() || ps.In("acme").LandsByPullRequest() {
		t.Errorf("one Repository under Pull Request: watcher %v, the Home by Pull Request %v; want the watcher on and the Home on Merge", ps.RunsPullRequestWatcher(), ps.In("acme").LandsByPullRequest())
	}
}
