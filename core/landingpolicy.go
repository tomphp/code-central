package core

import (
	"fmt"
	"slices"

	"github.com/tomoram/code-central/store"
)

// LandingPolicy is how one Repository's Landing Method and Push, and the
// Project's Land per Spec and CI Tracking, combine into the landing rules
// there. It is a pure value: it is built from the normalised Landing, so
// Pull Request with Push on reads as Push off and can't reach any rule.
type LandingPolicy struct {
	method      store.LandingMethod
	push        bool
	landPerSpec bool
	tracking    bool
}

// NewLandingPolicy is the policy of a Repository with these four inputs.
func NewLandingPolicy(method store.LandingMethod, push, landPerSpec, ciTracking bool) LandingPolicy {
	l := store.Landing{Method: method, Push: push}.Normalized()
	return LandingPolicy{method: l.Method, push: l.Push, landPerSpec: landPerSpec, tracking: ciTracking}
}

// LandingPolicies are the Landing Policies of a Project's Repositories, by
// Repository Name (Spec code-central-890). Per-Repository rules ask In,
// for the Repository in question; Project-wide rules ask across them.
type LandingPolicies struct {
	byName map[string]LandingPolicy
	// names are the Repository Names in landing order.
	names []string
	// fallback is the Default Landing's policy, for a name not listed.
	fallback LandingPolicy
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
func repositoryLandingOf(method store.LandingMethod, push bool) RepositoryLanding {
	switch {
	case method == store.LandingPullRequest:
		return RepositoryLandingPullRequest
	case method == store.LandingBranch && push:
		return RepositoryLandingBranchPush
	case method == store.LandingBranch:
		return RepositoryLandingBranch
	case push:
		return RepositoryLandingMergePush
	}
	return RepositoryLandingMerge
}

// methodAndPush is l's Landing Method and Push option; Merge for the
// default, which a view never holds.
func (l RepositoryLanding) methodAndPush() (store.LandingMethod, bool) {
	switch l {
	case RepositoryLandingMergePush:
		return store.LandingMerge, true
	case RepositoryLandingBranch:
		return store.LandingBranch, false
	case RepositoryLandingBranchPush:
		return store.LandingBranch, true
	case RepositoryLandingPullRequest:
		return store.LandingPullRequest, false
	}
	return store.LandingMerge, false
}

// ownLanding is l as a stored Own Landing: none for the default.
func (l RepositoryLanding) ownLanding() store.OwnLanding {
	if l == RepositoryLandingDefault {
		return store.OwnLanding{}
	}
	m, push := l.methodAndPush()
	return store.OwnLanding{Method: m, Push: push}
}

// effectiveLanding is r's Landing in p, its Own Landing or else p's
// Default Landing, and whether it is its own.
func effectiveLanding(p store.Project, r store.Repository) (RepositoryLanding, bool) {
	if o := r.OwnLanding; o.Set() {
		return repositoryLandingOf(o.Method, o.Push), true
	}
	return repositoryLandingOf(p.Landing.Method, p.Landing.Push), false
}

// repositoryLandingPolicies is the policies of repositories, given the
// Default Landing's method and Push and the Project's Land per Spec and CI
// Tracking. It and NewLandingPolicy are the only places that turn
// settings fields into a policy: a Repository's policy is the Landing
// Policy's "its Own Landing, or else the Default Landing, with Push off
// when the Repository is local-only".
func repositoryLandingPolicies(repositories []RepositoryView, method store.LandingMethod, push, landPerSpec, ciTracking bool) LandingPolicies {
	ps := LandingPolicies{byName: map[string]LandingPolicy{}, fallback: NewLandingPolicy(method, push, landPerSpec, ciTracking)}
	for _, r := range repositories {
		m, p := r.Landing.methodAndPush()
		ps.byName[r.Name] = NewLandingPolicy(m, p && !r.LocalOnly, landPerSpec, ciTracking)
		ps.names = append(ps.names, r.Name)
	}
	return ps
}

// markLocalOnlyUnderPullRequest flags each of vs, p's Repositories'
// views, that is local-only with a policy that needs a Clone URL: only a
// Home that lost its remote can be, and it keeps its Landing.
func markLocalOnlyUnderPullRequest(p store.Project, vs []RepositoryView) {
	ps := repositoryLandingPolicies(vs, p.Landing.Method, p.Landing.Push, p.Landing.LandPerSpec, p.CI.Tracking)
	for i, v := range vs {
		vs[i].LocalOnlyUnderPullRequest = v.LocalOnly && ps.In(v.Name).NeedsCloneURL()
	}
}

// landingPolicies is the policies of p's Repositories.
func landingPolicies(p store.Project) LandingPolicies {
	return repositoryLandingPolicies(repositoriesView(p), p.Landing.Method, p.Landing.Push, p.Landing.LandPerSpec, p.CI.Tracking)
}

// LandingPolicies is the policies of the view's Repositories.
func (p ProjectView) LandingPolicies() LandingPolicies {
	return repositoryLandingPolicies(p.Repositories, p.Landing.Method, p.Landing.Push, p.Landing.LandPerSpec, p.CI.Tracking)
}

// landingPolicyIn is the Landing Policy of p's Repository name, which
// every rule about one Repository's landing asks.
func landingPolicyIn(p store.Project, name string) LandingPolicy {
	return landingPolicies(p).In(name)
}

// In is the Landing Policy of the Repository name. A name not listed has
// the Default Landing's.
func (ps LandingPolicies) In(name string) LandingPolicy {
	if lp, ok := ps.byName[name]; ok {
		return lp
	}
	return ps.fallback
}

// ticketLandingPolicies is the policies of t's Changed Repositories in p,
// which a rule about t's landing asks across: the Home's while it has
// none.
func ticketLandingPolicies(p store.Project, t store.Ticket) LandingPolicies {
	names := changedRepositoryNames(p.Key, t)
	if len(names) == 0 {
		names = []string{p.Key}
	}
	return landingPolicies(p).Only(names)
}

// Only is ps narrowed to the Repositories names, such as a Ticket's
// Changed Repositories.
func (ps LandingPolicies) Only(names []string) LandingPolicies {
	out := LandingPolicies{byName: map[string]LandingPolicy{}, fallback: ps.fallback}
	for _, n := range names {
		out.byName[n] = ps.In(n)
		out.names = append(out.names, n)
	}
	return out
}

// Any reports whether rule holds in any Repository's policy, as a
// Project-wide rule asks: "Project-wide rules hold when any Repository's
// policy calls for them".
func (ps LandingPolicies) Any(rule func(LandingPolicy) bool) bool {
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
	return ps.Any(LandingPolicy.LandsByPullRequest)
}

// RunsCIWatcher answers the CI Watcher's "The service starts one per
// Project with CI Tracking on", which every Repository's policy shares.
func (ps LandingPolicies) RunsCIWatcher() bool {
	return ps.Any(LandingPolicy.RunsCIWatcher)
}

// UsesAzureReposPAT answers the PAT variable's "shown while any Azure
// Repos Repository is under Pull Request", given the Azure Repos
// Repositories' names.
func (ps LandingPolicies) UsesAzureReposPAT(azureRepos []string) bool {
	return ps.Only(azureRepos).Any(LandingPolicy.LandsByPullRequest)
}

// LandPerSpecInForce answers whether Land per Spec covers the Project's
// Specs: in force in any Repository's policy.
func (ps LandingPolicies) LandPerSpecInForce() bool {
	return ps.Any(LandingPolicy.LandPerSpecInForce)
}

// LandingMethodCapabilities is what the Options form offers with one
// Landing Method drafted as the Default Landing: its Landing
// capabilities.
type LandingMethodCapabilities struct {
	// OffersPush is whether the Push checkbox is offered.
	OffersPush bool `json:"offers_push"`
	// OffersLandPerSpec is whether Land per Spec can be changed.
	OffersLandPerSpec bool `json:"offers_land_per_spec"`
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

// landingCapabilities is p's Landing capabilities for each Landing Method
// as if it were the Default Landing, with p's other settings and every
// Own Landing unchanged, given the Azure Repos Repositories' names.
func landingCapabilities(p store.Project, azureRepos []string) LandingMethodsCapabilities {
	capabilitiesOf := func(m store.LandingMethod) LandingMethodCapabilities {
		drafted := p
		drafted.Landing.Method = m
		lp := NewLandingPolicy(m, p.Landing.Push, p.Landing.LandPerSpec, p.CI.Tracking)
		return LandingMethodCapabilities{OffersPush: lp.OffersPush(), OffersLandPerSpec: lp.OffersLandPerSpec(),
			ShowsCodeHost: lp.LandsByPullRequest(), UsesAzureReposPAT: landingPolicies(drafted).UsesAzureReposPAT(azureRepos)}
	}
	return LandingMethodsCapabilities{Merge: capabilitiesOf(store.LandingMerge), Branch: capabilitiesOf(store.LandingBranch),
		PullRequest: capabilitiesOf(store.LandingPullRequest)}
}

// pushesNothing is whether Push pushes nothing in p: every Repository is
// local-only while some Repository's Landing, as the human chose it, needs
// a push remote. It asks the chosen Landing's policy, not the
// Repository's: that has "Push off when the Repository is local-only", so
// would never need one here.
func pushesNothing(p store.Project) bool {
	needsRemote := false
	for _, r := range repositoriesView(p) {
		if !r.LocalOnly {
			return false
		}
		m, push := r.Landing.methodAndPush()
		needsRemote = needsRemote || NewLandingPolicy(m, push, p.Landing.LandPerSpec, p.CI.Tracking).NeedsPushRemote()
	}
	return needsRemote
}

// OffersPush answers Push's "Under Pull Request, Push is implied", so the
// human chooses it only under Merge and Branch.
func (lp LandingPolicy) OffersPush() bool {
	return lp.method != store.LandingPullRequest
}

// OffersLandPerSpec answers "Land per Spec applies under Merge and Pull
// Request, and has no effect under Branch": Merge or Pull Request,
// whatever the Land per Spec setting, unlike LandPerSpecInForce.
func (lp LandingPolicy) OffersLandPerSpec() bool {
	return lp.method == store.LandingMerge || lp.method == store.LandingPullRequest
}

// NeedsCloneURL answers Pull Request's "it is refused as a local-only
// Repository's Own Landing, and as the Default Landing while a local-only
// Repository follows it": Pull Request, which can't land where there is
// no Clone URL.
func (lp LandingPolicy) NeedsCloneURL() bool {
	return lp.method == store.LandingPullRequest
}

// LandPerSpecInForce answers "Land per Spec applies under Merge and Pull
// Request, and has no effect under Branch": Merge or Pull Request, with
// Land per Spec on.
func (lp LandingPolicy) LandPerSpecInForce() bool {
	return (lp.method == store.LandingMerge || lp.method == store.LandingPullRequest) && lp.landPerSpec
}

// CIGateHolds answers "Under Merge + Push with CI Tracking on, a Ticket
// lands only once its CI result is passed, at the pushed commit that
// passed": Merge and Push and CI Tracking.
func (lp LandingPolicy) CIGateHolds() bool {
	return lp.method == store.LandingMerge && lp.push && lp.tracking
}

// RecordsPushedCommit answers whether a pushed commit is kept on its
// Ticket: under Pull Request it is, for its pull request; otherwise only
// with Push and CI Tracking, for its CI result.
func (lp LandingPolicy) RecordsPushedCommit() bool {
	return lp.method == store.LandingPullRequest || (lp.push && lp.tracking)
}

// TracksCIResults answers whether a pushed commit has CI results: "Under
// Pull Request without CI Tracking a push is recorded for its pull request
// alone: it has no CI result". It is not the CI gate: Pull Request with CI
// Tracking has CI results but no gate. Nor is it RunsCIWatcher, which is
// CI Tracking alone: under Merge or Branch without CI Tracking a pushed
// commit still has CI results, though no CI Watcher runs to report them.
func (lp LandingPolicy) TracksCIResults() bool {
	return !(lp.method == store.LandingPullRequest && !lp.tracking)
}

// NeedsPushRemote answers whether the Factory pushes to a remote: Push is
// on, or "Under Pull Request, Push is implied".
func (lp LandingPolicy) NeedsPushRemote() bool {
	return lp.push || lp.method == store.LandingPullRequest
}

// PushesBase answers "the Factory pushes the base after a Merge": Merge
// and Push.
func (lp LandingPolicy) PushesBase() bool {
	return lp.method == store.LandingMerge && lp.push
}

// KeepsBaseLevel answers whether the Factory fetches the Push remote and
// fast-forwards the local base before it lands or cuts a branch, and
// reports Base diverged when it can't: Pull Request, or Merge and Push.
func (lp LandingPolicy) KeepsBaseLevel() bool {
	return lp.method == store.LandingPullRequest || lp.PushesBase()
}

// LandsOnHumanMerge answers whether a Ticket lands when a human merges it,
// so it needs them to merge: Branch (the human merges the branch) or Pull
// Request (the human merges the pull request). This is the To merge rule.
func (lp LandingPolicy) LandsOnHumanMerge() bool {
	return lp.method == store.LandingBranch || lp.method == store.LandingPullRequest
}

// OffersLandAnyway answers Land anyway's "Offered under Merge + Push only:
// under Branch + Push and under Pull Request the human's merge decides":
// Merge and Push. Under Merge without Push it isn't offered, though no CI
// item could offer it there anyway: no pushed commit is recorded.
func (lp LandingPolicy) OffersLandAnyway() bool {
	return lp.method == store.LandingMerge && lp.push
}

// FlagsNotLanded answers Accepted but not landed's "an Accepted Ticket,
// under Merge, that has not landed after a while": Merge.
func (lp LandingPolicy) FlagsNotLanded() bool {
	return lp.method == store.LandingMerge
}

// LandsByPullRequest answers Pull Request's "the Ticket lands when the
// host reports it merged": Pull Request.
func (lp LandingPolicy) LandsByPullRequest() bool {
	return lp.method == store.LandingPullRequest
}

// RunsCIWatcher answers the CI Watcher's "The service starts one per
// Project with CI Tracking on": CI Tracking, whatever the method. It is
// not TracksCIResults, which says whether a pushed commit has CI results
// and is true without CI Tracking too, except under Pull Request. The
// Project asks it across its Repositories, as LandingPolicies does.
func (lp LandingPolicy) RunsCIWatcher() bool {
	return lp.tracking
}

// MergesSpecBranches answers the Spec Branch's "the Factory merges it into
// the base once the Spec is ready to land": Merge, with Land per Spec in
// force.
func (lp LandingPolicy) MergesSpecBranches() bool {
	return lp.method == store.LandingMerge && lp.LandPerSpecInForce()
}

// PushesSpecBranches answers the Spec Branch's "It is pushed only under
// Pull Request, where the Spec's one pull request is opened from it": Pull
// Request, with Land per Spec in force.
func (lp LandingPolicy) PushesSpecBranches() bool {
	return lp.method == store.LandingPullRequest && lp.LandPerSpecInForce()
}

// MergesLeftoverSpecBranches answers Land per Spec's "turning it off …
// lands the Spec's waiting Tickets together at once, ready or not": Merge,
// where the Factory merges a Spec Branch Land per Spec no longer covers.
func (lp LandingPolicy) MergesLeftoverSpecBranches() bool {
	return lp.method == store.LandingMerge
}

// PushesAcceptedBranch answers Push's "the Ticket's branch on accept under
// Branch": Branch and Push.
func (lp LandingPolicy) PushesAcceptedBranch() bool {
	return lp.method == store.LandingBranch && lp.push
}

// KeepsAcceptedWorktree answers whether the Factory keeps an Accepted
// Ticket's worktree, because it still lands the Ticket itself: Merge or
// Pull Request. Under Branch the human merges the branch, so the Factory
// is done with it. A Spec-landed Ticket whose merge into its Spec Branch
// counts is done with too, but that is a fact of the Ticket, not of the
// policy.
func (lp LandingPolicy) KeepsAcceptedWorktree() bool {
	return lp.method != store.LandingBranch
}

// LandingRoute is the way the Factory lands one Accepted Ticket.
type LandingRoute int

const (
	// RouteBranch waits for the human to merge the Ticket's branch.
	RouteBranch LandingRoute = iota
	// RoutePullRequest lands the Ticket by its pull request.
	RoutePullRequest
	// RouteSpecBranch merges the Ticket into its Spec Branch.
	RouteSpecBranch
	// RouteMerge merges the Ticket into the base.
	RouteMerge
)

// String names the route.
func (r LandingRoute) String() string {
	switch r {
	case RouteBranch:
		return "Branch"
	case RoutePullRequest:
		return "Pull Request"
	case RouteSpecBranch:
		return "Spec Branch"
	case RouteMerge:
		return "Merge"
	}
	return fmt.Sprintf("LandingRoute(%d)", int(r))
}

// Route is how the Factory lands an Accepted Ticket, given whether it
// goes through its Spec Branch (it is Spec-landed and its work isn't in
// the base) and whether its pull request is merged. Under Branch it
// waits for the human's merge, whatever the facts; under Pull Request it
// lands by its pull request once that is merged or when it isn't on a
// Spec Branch; otherwise it goes through its Spec Branch when on one, or
// is merged into the base.
func (lp LandingPolicy) Route(onSpecBranch, prMerged bool) LandingRoute {
	switch {
	case lp.method == store.LandingBranch:
		return RouteBranch
	case lp.method == store.LandingPullRequest && (prMerged || !onSpecBranch):
		return RoutePullRequest
	case onSpecBranch:
		return RouteSpecBranch
	}
	return RouteMerge
}
