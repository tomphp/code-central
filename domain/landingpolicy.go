package domain

import "fmt"

// LandingPolicy is how one Repository's Landing Method and Push, and the
// Project's Land per Spec, Rebase and CI Tracking, combine into the
// landing rules there. It is a pure value: it is built from the
// normalised Landing, so Pull Request with Push on reads as Push off and
// can't reach any rule.
type LandingPolicy struct {
	method      LandingMethod
	push        bool
	landPerSpec bool
	rebase      bool
	tracking    bool
}

// NewLandingPolicy is the policy of a Repository with these five inputs.
func NewLandingPolicy(method LandingMethod, push, landPerSpec, rebase, ciTracking bool) LandingPolicy {
	l := Landing{Method: method, Push: push}.Normalized()
	return LandingPolicy{method: l.Method, push: l.Push, landPerSpec: landPerSpec, rebase: rebase, tracking: ciTracking}
}

// OffersPush answers Push's "Under Pull Request, Push is implied", so the
// human chooses it only under Merge and Branch.
func (lp LandingPolicy) OffersPush() bool {
	return lp.method != LandingPullRequest
}

// OffersLandPerSpec answers "Land per Spec applies under Merge and Pull
// Request, and has no effect under Branch": Merge or Pull Request,
// whatever the Land per Spec setting, unlike LandPerSpecInForce.
func (lp LandingPolicy) OffersLandPerSpec() bool {
	return lp.method == LandingMerge || lp.method == LandingPullRequest
}

// NeedsCloneURL answers Pull Request's "it is refused as a local-only
// Repository's Own Landing, and as the Default Landing while a local-only
// Repository follows it": Pull Request, which can't land where there is
// no Clone URL.
func (lp LandingPolicy) NeedsCloneURL() bool {
	return lp.method == LandingPullRequest
}

// LandPerSpecInForce answers "Land per Spec applies under Merge and Pull
// Request, and has no effect under Branch": Merge or Pull Request, with
// Land per Spec on.
func (lp LandingPolicy) LandPerSpecInForce() bool {
	return (lp.method == LandingMerge || lp.method == LandingPullRequest) && lp.landPerSpec
}

// OffersRebase answers "Rebase applies under Merge and Pull Request":
// Merge or Pull Request, whatever the Rebase setting, unlike
// RebaseInForce.
func (lp LandingPolicy) OffersRebase() bool {
	return lp.method == LandingMerge || lp.method == LandingPullRequest
}

// RebaseInForce answers whether the Factory replays a Ticket's commits
// onto its target instead of merging its branch: Rebase offered, and on.
func (lp LandingPolicy) RebaseInForce() bool {
	return lp.OffersRebase() && lp.rebase
}

// CIGateHolds answers "Under Merge + Push with CI Tracking on, a Ticket
// lands only once its CI result is passed, at the pushed commit that
// passed": Merge and Push and CI Tracking.
func (lp LandingPolicy) CIGateHolds() bool {
	return lp.method == LandingMerge && lp.push && lp.tracking
}

// RecordsPushedCommit answers whether a pushed commit is kept on its
// Ticket: under Pull Request it is, for its pull request; otherwise only
// with Push and CI Tracking, for its CI result.
func (lp LandingPolicy) RecordsPushedCommit() bool {
	return lp.method == LandingPullRequest || (lp.push && lp.tracking)
}

// TracksCIResults answers whether a pushed commit has CI results: "Under
// Pull Request without CI Tracking a push is recorded for its pull request
// alone: it has no CI result". It is not the CI gate: Pull Request with CI
// Tracking has CI results but no gate. Nor is it RunsCIWatcher, which is
// CI Tracking alone: under Merge or Branch without CI Tracking a pushed
// commit still has CI results, though no CI Watcher runs to report them.
func (lp LandingPolicy) TracksCIResults() bool {
	return !(lp.method == LandingPullRequest && !lp.tracking)
}

// NeedsPushRemote answers whether the Factory pushes to a remote: Push is
// on, or "Under Pull Request, Push is implied".
func (lp LandingPolicy) NeedsPushRemote() bool {
	return lp.push || lp.method == LandingPullRequest
}

// PushesBase answers "the Factory pushes the base after a Merge": Merge
// and Push.
func (lp LandingPolicy) PushesBase() bool {
	return lp.method == LandingMerge && lp.push
}

// KeepsBaseLevel answers whether the Factory fetches the Push remote and
// fast-forwards the local base before it lands or cuts a branch, and
// reports Base diverged when it can't: Pull Request, or Merge and Push.
func (lp LandingPolicy) KeepsBaseLevel() bool {
	return lp.method == LandingPullRequest || lp.PushesBase()
}

// LandsOnHumanMerge answers whether a Ticket lands when a human merges it,
// so it needs them to merge: Branch (the human merges the branch) or Pull
// Request (the human merges the pull request). This is the To merge rule.
func (lp LandingPolicy) LandsOnHumanMerge() bool {
	return lp.method == LandingBranch || lp.method == LandingPullRequest
}

// OffersLandAnyway answers Land anyway's "Offered under Merge + Push only:
// under Branch + Push and under Pull Request the human's merge decides":
// Merge and Push. Under Merge without Push it isn't offered, though no CI
// item could offer it there anyway: no pushed commit is recorded.
func (lp LandingPolicy) OffersLandAnyway() bool {
	return lp.method == LandingMerge && lp.push
}

// FlagsNotLanded answers Accepted but not landed's "an Accepted Ticket,
// under Merge, that has not landed after a while": Merge.
func (lp LandingPolicy) FlagsNotLanded() bool {
	return lp.method == LandingMerge
}

// LandsByPullRequest answers Pull Request's "the Ticket lands when the
// host reports it merged": Pull Request.
func (lp LandingPolicy) LandsByPullRequest() bool {
	return lp.method == LandingPullRequest
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
	return lp.method == LandingMerge && lp.LandPerSpecInForce()
}

// PushesSpecBranches answers the Spec Branch's "It is pushed only under
// Pull Request, where the Spec's one pull request is opened from it": Pull
// Request, with Land per Spec in force.
func (lp LandingPolicy) PushesSpecBranches() bool {
	return lp.method == LandingPullRequest && lp.LandPerSpecInForce()
}

// MergesLeftoverSpecBranches answers Land per Spec's "turning it off …
// lands the Spec's waiting Tickets together at once, ready or not": Merge,
// where the Factory merges a Spec Branch Land per Spec no longer covers.
func (lp LandingPolicy) MergesLeftoverSpecBranches() bool {
	return lp.method == LandingMerge
}

// PushesAcceptedBranch answers Push's "the Ticket's branch on accept under
// Branch": Branch and Push.
func (lp LandingPolicy) PushesAcceptedBranch() bool {
	return lp.method == LandingBranch && lp.push
}

// KeepsAcceptedWorktree answers whether the Factory keeps an Accepted
// Ticket's worktree, because it still lands the Ticket itself: Merge or
// Pull Request. Under Branch the human merges the branch, so the Factory
// is done with it. A Spec-landed Ticket whose merge into its Spec Branch
// counts is done with too, but that is a fact of the Ticket, not of the
// policy.
func (lp LandingPolicy) KeepsAcceptedWorktree() bool {
	return lp.method != LandingBranch
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
	case lp.method == LandingBranch:
		return RouteBranch
	case lp.method == LandingPullRequest && (prMerged || !onSpecBranch):
		return RoutePullRequest
	case onSpecBranch:
		return RouteSpecBranch
	}
	return RouteMerge
}
