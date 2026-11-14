package core

import "github.com/tomoram/code-central/store"

// LandingPolicy is how a Project's Landing Method, Push, Land per Spec and
// CI Tracking combine into the landing rules. It is a pure value: it is
// built from the normalised Landing, so Pull Request with Push on reads as
// Push off and can't reach any rule.
type LandingPolicy struct {
	method      store.LandingMethod
	push        bool
	landPerSpec bool
	tracking    bool
}

// NewLandingPolicy is the policy of a Project with these four inputs.
func NewLandingPolicy(method store.LandingMethod, push, landPerSpec, ciTracking bool) LandingPolicy {
	l := store.Landing{Method: method, Push: push}.Normalized()
	return LandingPolicy{method: l.Method, push: l.Push, landPerSpec: landPerSpec, tracking: ciTracking}
}

// landingPolicyOf is the policy of p's Landing and CI Tracking. It and
// NewLandingPolicy are the only places that turn settings fields into a
// policy.
func landingPolicyOf(p store.Project) LandingPolicy {
	return NewLandingPolicy(p.Landing.Method, p.Landing.Push, p.Landing.LandPerSpec, p.CI.Tracking)
}

// LandingPolicy is the policy of the view's Landing and CI Tracking.
func (p ProjectView) LandingPolicy() LandingPolicy {
	return NewLandingPolicy(p.Landing.Method, p.Landing.Push, p.Landing.LandPerSpec, p.CI.Tracking)
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
// Tracking has CI results but no gate.
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
