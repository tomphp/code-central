package api

import (
	"slices"

	"github.com/tomoram/code-central/domain"
)

// RepositoryLanding is a Repository's Landing as the human chooses it:
// the Project's Default Landing, or an Own Landing of a Landing Method
// with its Push option. A Repository's view shows its effective one,
// never RepositoryLandingDefault.
type RepositoryLanding string

// RepositoryLandings lists every RepositoryLanding, the default first.
func RepositoryLandings() []RepositoryLanding {
	return []RepositoryLanding{RepositoryLandingDefault, RepositoryLandingMerge, RepositoryLandingMergePush,
		RepositoryLandingBranch, RepositoryLandingBranchPush, RepositoryLandingPullRequest}
}

// Valid reports whether l is one of the RepositoryLandings.
func (l RepositoryLanding) Valid() bool { return slices.Contains(RepositoryLandings(), l) }

// LandingMethodCapabilities is what the Options form offers with one
// Landing Method drafted as the Default Landing: its Landing
// capabilities.
type LandingMethodCapabilities struct {
	// OffersPush is whether the Push checkbox is offered.
	OffersPush bool `json:"offers_push"`
	// OffersLandPerSpec is whether Land per Spec can be changed.
	OffersLandPerSpec bool `json:"offers_land_per_spec"`
	// OffersRebase is whether Rebase can be changed.
	OffersRebase bool `json:"offers_rebase"`
	// ShowsCodeHost is whether the Code Host worked out from the Push
	// remote URL is shown.
	ShowsCodeHost bool `json:"shows_code_host"`
	// UsesAzureReposPAT is whether the PAT variable is used.
	UsesAzureReposPAT bool `json:"uses_azure_repos_pat"`
}

// LandingMethodsCapabilities is the Landing capabilities of every Landing
// Method, each as if it were the Default Landing.
type LandingMethodsCapabilities struct {
	// Merge is what the form offers with Merge drafted.
	Merge LandingMethodCapabilities `json:"merge"`
	// Branch is what the form offers with Leave as a branch drafted.
	Branch LandingMethodCapabilities `json:"branch"`
	// PullRequest is what the form offers with Pull Request drafted.
	PullRequest LandingMethodCapabilities `json:"pull_request"`
}

// The Repository Landings.
const (
	RepositoryLandingDefault     RepositoryLanding = "default"
	RepositoryLandingMerge       RepositoryLanding = "merge"
	RepositoryLandingMergePush   RepositoryLanding = "merge_push"
	RepositoryLandingBranch      RepositoryLanding = "branch"
	RepositoryLandingBranchPush  RepositoryLanding = "branch_push"
	RepositoryLandingPullRequest RepositoryLanding = "pull_request"
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

// MethodAndPush is l's Landing Method and Push option; Merge for the
// default, which a view never holds.
func MethodAndPush(l RepositoryLanding) (domain.LandingMethod, bool) {
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

// NewLandingPolicies is the policies of repositories, given the
// Default Landing's method and Push and the Project's Land per Spec,
// Rebase and CI Tracking. It and domain.NewLandingPolicy are the only
// places that turn settings fields into a policy: a Repository's policy
// is the Landing Policy's "its Own Landing, or else the Default Landing,
// with Push off when the Repository is local-only". Land per Spec and
// Rebase are Project-wide: an Own Landing is method and Push alone.
func NewLandingPolicies(repositories []RepositoryView, method domain.LandingMethod, push, landPerSpec, rebase, ciTracking bool) LandingPolicies {
	ps := LandingPolicies{byName: map[string]domain.LandingPolicy{}, fallback: domain.NewLandingPolicy(method, push, landPerSpec, rebase, ciTracking)}
	for _, r := range repositories {
		m, p := MethodAndPush(r.Landing)
		ps.byName[r.Name] = domain.NewLandingPolicy(m, p && !r.LocalOnly, landPerSpec, rebase, ciTracking)
		ps.names = append(ps.names, r.Name)
	}
	return ps
}

// LandingPolicies is the policies of the view's Repositories.
func (p ProjectView) LandingPolicies() LandingPolicies {
	return NewLandingPolicies(p.Repositories, p.Landing.Method, p.Landing.Push, p.Landing.LandPerSpec, p.Landing.Rebase, p.CI.Tracking)
}

// In is the Landing Policy of the Repository name. A name not listed has
// the Default Landing's.
func (ps LandingPolicies) In(name string) domain.LandingPolicy {
	if lp, ok := ps.byName[name]; ok {
		return lp
	}
	return ps.fallback
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

// RebaseInForce answers whether Rebase covers any of the Project's
// landings: in force in any Repository's policy.
func (ps LandingPolicies) RebaseInForce() bool {
	return ps.Any(domain.LandingPolicy.RebaseInForce)
}
