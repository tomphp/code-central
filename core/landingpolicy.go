package core

import (
	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// repositoryLandingOf is the RepositoryLanding of a Landing Method with
// its Push option, normalised: Pull Request implies Push.
func repositoryLandingOf(method domain.LandingMethod, push bool) api.RepositoryLanding {
	switch {
	case method == domain.LandingPullRequest:
		return api.RepositoryLandingPullRequest
	case method == domain.LandingBranch && push:
		return api.RepositoryLandingBranchPush
	case method == domain.LandingBranch:
		return api.RepositoryLandingBranch
	case push:
		return api.RepositoryLandingMergePush
	}
	return api.RepositoryLandingMerge
}

// ownLanding is l as a stored Own Landing: none for the default.
func ownLanding(l api.RepositoryLanding) domain.OwnLanding {
	if l == api.RepositoryLandingDefault {
		return domain.OwnLanding{}
	}
	m, push := api.MethodAndPush(l)
	return domain.OwnLanding{Method: m, Push: push}
}

// effectiveLanding is r's Landing in p, its Own Landing or else p's
// Default Landing, and whether it is its own.
func effectiveLanding(p domain.Project, r domain.Repository) (api.RepositoryLanding, bool) {
	if o := r.OwnLanding; o.Set() {
		return repositoryLandingOf(o.Method, o.Push), true
	}
	return repositoryLandingOf(p.Landing.Method, p.Landing.Push), false
}

// markLocalOnlyUnderPullRequest flags each of vs, p's Repositories'
// views, that is local-only with a policy that needs a Clone URL: only a
// Home that lost its remote can be, and it keeps its Landing.
func markLocalOnlyUnderPullRequest(p domain.Project, vs []api.RepositoryView) {
	ps := api.NewLandingPolicies(vs, p.Landing.Method, p.Landing.Push, p.Landing.LandPerSpec, p.Landing.Rebase, p.CI.Tracking)
	for i, v := range vs {
		vs[i].LocalOnlyUnderPullRequest = v.LocalOnly && ps.In(v.Name).NeedsCloneURL()
	}
}

// landingPolicies is the policies of p's Repositories.
func landingPolicies(p domain.Project) api.LandingPolicies {
	return api.NewLandingPolicies(repositoriesView(p), p.Landing.Method, p.Landing.Push, p.Landing.LandPerSpec, p.Landing.Rebase, p.CI.Tracking)
}

// landingPolicyIn is the Landing Policy of p's Repository name, which
// every rule about one Repository's landing asks.
func landingPolicyIn(p domain.Project, name string) domain.LandingPolicy {
	return landingPolicies(p).In(name)
}

// ticketLandingPolicies is the policies of t's Changed Repositories in p,
// which a rule about t's landing asks across: the Home's while it has
// none.
func ticketLandingPolicies(p domain.Project, t domain.Ticket) api.LandingPolicies {
	names := changedRepositoryNames(p.Key, t)
	if len(names) == 0 {
		names = []string{p.Key}
	}
	return landingPolicies(p).Only(names)
}

// landingCapabilities is p's Landing capabilities for each Landing Method
// as if it were the Default Landing, with p's other settings and every
// Own Landing unchanged, given the Azure Repos Repositories' names.
func landingCapabilities(p domain.Project, azureRepos []string) api.LandingMethodsCapabilities {
	capabilitiesOf := func(m domain.LandingMethod) api.LandingMethodCapabilities {
		drafted := p
		drafted.Landing.Method = m
		lp := domain.NewLandingPolicy(m, p.Landing.Push, p.Landing.LandPerSpec, p.Landing.Rebase, p.CI.Tracking)
		return api.LandingMethodCapabilities{OffersPush: lp.OffersPush(), OffersLandPerSpec: lp.OffersLandPerSpec(),
			OffersRebase: lp.OffersRebase(), ShowsCodeHost: lp.LandsByPullRequest(), UsesAzureReposPAT: landingPolicies(drafted).UsesAzureReposPAT(azureRepos)}
	}
	return api.LandingMethodsCapabilities{Merge: capabilitiesOf(domain.LandingMerge), Branch: capabilitiesOf(domain.LandingBranch),
		PullRequest: capabilitiesOf(domain.LandingPullRequest)}
}

// pushesNothing is whether Push pushes nothing in p: every Repository is
// local-only while some Repository's Landing, as the human chose it, needs
// a push remote. It asks the chosen Landing's policy, not the
// Repository's: that has "Push off when the Repository is local-only", so
// would never need one here.
func pushesNothing(p domain.Project) bool {
	needsRemote := false
	for _, r := range repositoriesView(p) {
		if !r.LocalOnly {
			return false
		}
		m, push := api.MethodAndPush(r.Landing)
		needsRemote = needsRemote || domain.NewLandingPolicy(m, push, p.Landing.LandPerSpec, p.Landing.Rebase, p.CI.Tracking).NeedsPushRemote()
	}
	return needsRemote
}
