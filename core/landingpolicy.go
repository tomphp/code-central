package core

import (
	"slices"

	"github.com/tomoram/code-central/domain"
)

// LandingPolicies are the Landing Policies of a Project's Repositories, by
// Repository Name (Spec code-central-890). Per-Repository rules ask In,
// for the Repository in question; Project-wide rules ask across them.
type LandingPolicies struct {
	byName map[string]domain.LandingPolicy
	// names are the Repository Names in landing order.
	names []string
	// fallback is the Default Landing's policy, for a name not listed.
	fallback domain.LandingPolicy
}

// RepositoryLanding is a Repository's Landing as the human chooses it:
// the Project's Default Landing, or an Own Landing of a Landing Method
// with its Push option. A Repository's view shows its effective one,
// never RepositoryLandingDefault.
type RepositoryLanding string

// The Repository Landings.
const (
	RepositoryLandingDefault     RepositoryLanding = "default"
	RepositoryLandingMerge       RepositoryLanding = "merge"
	RepositoryLandingMergePush   RepositoryLanding = "merge_push"
	RepositoryLandingBranch      RepositoryLanding = "branch"
	RepositoryLandingBranchPush  RepositoryLanding = "branch_push"
	RepositoryLandingPullRequest RepositoryLanding = "pull_request"
)

// RepositoryLandings lists every RepositoryLanding, the default first.
func RepositoryLandings() []RepositoryLanding {
	return []RepositoryLanding{RepositoryLandingDefault, RepositoryLandingMerge, RepositoryLandingMergePush,
		RepositoryLandingBranch, RepositoryLandingBranchPush, RepositoryLandingPullRequest}
}

// Valid reports whether l is one of the RepositoryLandings.
func (l RepositoryLanding) Valid() bool { return slices.Contains(RepositoryLandings(), l) }

// repositoryLandingOf is the RepositoryLanding of a Landing Method with
// its Push option, normalised: Pull Request implies Push.
func repositoryLandingOf(method domain.LandingMethod, push bool) RepositoryLanding {
	switch {
	case method == domain.LandingPullRequest:
		return RepositoryLandingPullRequest
	case method == domain.LandingBranch && push:
		return RepositoryLandingBranchPush
	case method == domain.LandingBranch:
		return RepositoryLandingBranch
	case push:
		return RepositoryLandingMergePush
	}
	return RepositoryLandingMerge
}

// methodAndPush is l's Landing Method and Push option; Merge for the
// default, which a view never holds.
func (l RepositoryLanding) methodAndPush() (domain.LandingMethod, bool) {
	switch l {
	case RepositoryLandingMergePush:
		return domain.LandingMerge, true
	case RepositoryLandingBranch:
		return domain.LandingBranch, false
	case RepositoryLandingBranchPush:
		return domain.LandingBranch, true
	case RepositoryLandingPullRequest:
		return domain.LandingPullRequest, false
	}
	return domain.LandingMerge, false
}

// ownLanding is l as a stored Own Landing: none for the default.
func (l RepositoryLanding) ownLanding() domain.OwnLanding {
	if l == RepositoryLandingDefault {
		return domain.OwnLanding{}
	}
	m, push := l.methodAndPush()
	return domain.OwnLanding{Method: m, Push: push}
}

// effectiveLanding is r's Landing in p, its Own Landing or else p's
// Default Landing, and whether it is its own.
func effectiveLanding(p domain.Project, r domain.Repository) (RepositoryLanding, bool) {
	if o := r.OwnLanding; o.Set() {
		return repositoryLandingOf(o.Method, o.Push), true
	}
	return repositoryLandingOf(p.Landing.Method, p.Landing.Push), false
}

// repositoryLandingPolicies is the policies of repositories, given the
// Default Landing's method and Push and the Project's Land per Spec and CI
// Tracking. It and domain.NewLandingPolicy are the only places that turn
// settings fields into a policy: a Repository's policy is the Landing
// Policy's "its Own Landing, or else the Default Landing, with Push off
// when the Repository is local-only".
func repositoryLandingPolicies(repositories []RepositoryView, method domain.LandingMethod, push, landPerSpec, ciTracking bool) LandingPolicies {
	ps := LandingPolicies{byName: map[string]domain.LandingPolicy{}, fallback: domain.NewLandingPolicy(method, push, landPerSpec, ciTracking)}
	for _, r := range repositories {
		m, p := r.Landing.methodAndPush()
		ps.byName[r.Name] = domain.NewLandingPolicy(m, p && !r.LocalOnly, landPerSpec, ciTracking)
		ps.names = append(ps.names, r.Name)
	}
	return ps
}

// markLocalOnlyUnderPullRequest flags each of vs, p's Repositories'
// views, that is local-only with a policy that needs a Clone URL: only a
// Home that lost its remote can be, and it keeps its Landing.
func markLocalOnlyUnderPullRequest(p domain.Project, vs []RepositoryView) {
	ps := repositoryLandingPolicies(vs, p.Landing.Method, p.Landing.Push, p.Landing.LandPerSpec, p.CI.Tracking)
	for i, v := range vs {
		vs[i].LocalOnlyUnderPullRequest = v.LocalOnly && ps.In(v.Name).NeedsCloneURL()
	}
}

// landingPolicies is the policies of p's Repositories.
func landingPolicies(p domain.Project) LandingPolicies {
	return repositoryLandingPolicies(repositoriesView(p), p.Landing.Method, p.Landing.Push, p.Landing.LandPerSpec, p.CI.Tracking)
}

// LandingPolicies is the policies of the view's Repositories.
func (p ProjectView) LandingPolicies() LandingPolicies {
	return repositoryLandingPolicies(p.Repositories, p.Landing.Method, p.Landing.Push, p.Landing.LandPerSpec, p.CI.Tracking)
}

// landingPolicyIn is the Landing Policy of p's Repository name, which
// every rule about one Repository's landing asks.
func landingPolicyIn(p domain.Project, name string) domain.LandingPolicy {
	return landingPolicies(p).In(name)
}

// In is the Landing Policy of the Repository name. A name not listed has
// the Default Landing's.
func (ps LandingPolicies) In(name string) domain.LandingPolicy {
	if lp, ok := ps.byName[name]; ok {
		return lp
	}
	return ps.fallback
}

// ticketLandingPolicies is the policies of t's Changed Repositories in p,
// which a rule about t's landing asks across: the Home's while it has
// none.
func ticketLandingPolicies(p domain.Project, t domain.Ticket) LandingPolicies {
	names := changedRepositoryNames(p.Key, t)
	if len(names) == 0 {
		names = []string{p.Key}
	}
	return landingPolicies(p).Only(names)
}

// Only is ps narrowed to the Repositories names, such as a Ticket's
// Changed Repositories.
func (ps LandingPolicies) Only(names []string) LandingPolicies {
	out := LandingPolicies{byName: map[string]domain.LandingPolicy{}, fallback: ps.fallback}
	for _, n := range names {
		out.byName[n] = ps.In(n)
		out.names = append(out.names, n)
	}
	return out
}

// Any reports whether rule holds in any Repository's policy, as a
// Project-wide rule asks: "Project-wide rules hold when any Repository's
// policy calls for them".
func (ps LandingPolicies) Any(rule func(domain.LandingPolicy) bool) bool {
	for _, n := range ps.names {
		if rule(ps.byName[n]) {
			return true
		}
	}
	return false
}

// RunsPullRequestWatcher answers the Pull Request Watcher's "The service
// runs one per Project while any Repository is under Pull Request".
func (ps LandingPolicies) RunsPullRequestWatcher() bool {
	return ps.Any(domain.LandingPolicy.LandsByPullRequest)
}

// RunsCIWatcher answers the CI Watcher's "The service starts one per
// Project with CI Tracking on", which every Repository's policy shares.
func (ps LandingPolicies) RunsCIWatcher() bool {
	return ps.Any(domain.LandingPolicy.RunsCIWatcher)
}

// UsesAzureReposPAT answers the PAT variable's "shown while any Azure
// Repos Repository is under Pull Request", given the Azure Repos
// Repositories' names.
func (ps LandingPolicies) UsesAzureReposPAT(azureRepos []string) bool {
	return ps.Only(azureRepos).Any(domain.LandingPolicy.LandsByPullRequest)
}

// LandPerSpecInForce answers whether Land per Spec covers the Project's
// Specs: in force in any Repository's policy.
func (ps LandingPolicies) LandPerSpecInForce() bool {
	return ps.Any(domain.LandingPolicy.LandPerSpecInForce)
}
