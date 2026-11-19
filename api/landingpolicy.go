package api

import (
	"slices"
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
