package api

import (
	"time"

	"github.com/tomoram/code-central/domain"
)

// NeedsMeReason is why a Ticket waits on the human. "" means it doesn't.
type NeedsMeReason string

// The reasons, in group order: cheapest to unblock first.
const (
	ReasonNeedsAnswer NeedsMeReason = "needs_answer"
	ReasonAnswered    NeedsMeReason = "answered"
	ReasonStopped     NeedsMeReason = "stopped"
	ReasonSilentClaim NeedsMeReason = "silent_claim"
	ReasonLongClaim   NeedsMeReason = "long_claim"
	ReasonFailed      NeedsMeReason = "failed"
	ReasonInReview    NeedsMeReason = "in_review"
	// ReasonCIFailed is a pushed commit whose CI result is failed, with
	// the Accepted Tickets waiting on it, which the human hasn't dealt
	// with (code-central-261). It is one item per commit, with no Ticket
	// (code-central-432); each listed Ticket reads it as its reason.
	ReasonCIFailed NeedsMeReason = "ci_failed"
	// ReasonCICancelled is a pushed commit whose CI result is cancelled,
	// with the Accepted Tickets waiting on it, which the human hasn't
	// dealt with. One item per commit, like CI failed.
	ReasonCICancelled NeedsMeReason = "ci_cancelled"
	// ReasonBaseDiverged is a Project under Pull Request or Merge + Push
	// whose Factory reports its local base diverged from the remote's
	// (code-central-394, code-central-425). It is one item per Project,
	// with no Ticket.
	ReasonBaseDiverged NeedsMeReason = "base_diverged"
	// ReasonBaseMissing is a Project whose Factory reports its base
	// missing, under any Landing Method. It is one item per Project, with
	// no Ticket.
	ReasonBaseMissing NeedsMeReason = "base_missing"
	// ReasonRepositoryMissing is a Repository whose Factory reports its
	// clone missing (Spec code-central-848): its path holds something
	// other than a clone of its Clone URL, or cloning it failed. It is one
	// item per Repository, with no Ticket.
	ReasonRepositoryMissing NeedsMeReason = "repository_missing"
	// ReasonPullRequestClosed is an Accepted Ticket under Pull Request
	// whose pull request is closed or abandoned without merging
	// (code-central-391); it clears if the pull request is reopened.
	ReasonPullRequestClosed NeedsMeReason = "pull_request_closed"
	// ReasonToMerge is a Ticket Accepted under the Branch or Pull Request
	// Landing Method (code-central-175): the human merges its branch or
	// pull request, at once.
	ReasonToMerge NeedsMeReason = "to_merge"
	// ReasonCIUnavailable is a pushed commit the CI gate has held Tickets
	// on for CIUnavailableAfter since their push with no word of its CI
	// (code-central-262). One item per commit, like CI failed.
	ReasonCIUnavailable NeedsMeReason = "ci_unavailable"
	// ReasonAcceptedNotLanded is a Ticket Accepted for more than
	// NotLandedAfter (ticket 72): its work hasn't reached the base.
	ReasonAcceptedNotLanded  NeedsMeReason = "accepted_not_landed"
	ReasonBlockedByCancelled NeedsMeReason = "blocked_by_cancelled"
	// ReasonNewIssue is an Incoming Issue awaiting triage that isn't
	// finished and isn't closed externally (code-central-287).
	ReasonNewIssue NeedsMeReason = "new_issue"
	// ReasonClosedExternally is a linked Ticket whose external item isn't
	// open, whose Link isn't detached and that isn't finished (ticket 42).
	ReasonClosedExternally NeedsMeReason = "closed_externally"
)

// NeedsMeView is needs_me: everything waiting on the human across every
// Project, in groups in group order (empty groups left out), each group
// longest-waiting first. Every view renders this order as it comes.
type NeedsMeView struct {
	// Count is the Needs Me count across every Project: the sum of
	// Counts.
	Count int `json:"count"`
	// Counts is the Needs Me count of each Project, left out while it has
	// nothing in Needs Me.
	Counts map[string]int `json:"counts"`
	Groups []NeedsMeGroup `json:"groups"`
}

// NeedsMeGroup is the Tickets waiting for one reason.
type NeedsMeGroup struct {
	Reason  NeedsMeReason `json:"reason"`
	Tickets []NeedsMeRow  `json:"tickets"`
}

// NeedsMeRow is one waiting Ticket with what its one-line detail needs.
// Since is when it started waiting: the Round posted, the last answer,
// the last activity (Silent Claim, so the quiet time is now minus it),
// the last move (Long Claim),
// the Failure Report, the finish, the first failed CI Run finishing (CI
// failed), the accept (To merge, and Accepted but not landed), the push
// (CI unavailable),
// the Blocker's cancel, or the Ticket's creation (New issue). Only the
// detail field for its reason is set. A CI failed, CI cancelled or CI
// unavailable row is one per pushed commit, with no Ticket: its ID is
// empty, and CIItem lists the Tickets waiting on the commit.
type NeedsMeRow struct {
	Project string        `json:"project"`
	ID      string        `json:"id"`
	Title   string        `json:"title"`
	State   domain.State  `json:"state"`
	Reason  NeedsMeReason `json:"reason"`
	Since   time.Time     `json:"since"`
	// FocusItem says the Project's Focus Kinds leave the row's reason on.
	// Focus Kinds mark rows and filter nothing.
	FocusItem bool `json:"focus_item"`
	// Round is the latest Round (Needs Answer).
	Round *RoundSummary `json:"round,omitempty"`
	// Failure is the latest Failure Report (Failed).
	Failure *FailureSummary `json:"failure,omitempty"`
	// Summary is the first line of the finish summary (In Review).
	Summary string `json:"summary,omitempty"`
	// Branch is the Ticket's branch, for the human to merge (To merge).
	Branch string `json:"branch,omitempty"`
	// CIFailure is what failed on the pushed commit (CI failed).
	CIFailure *CIFailureSummary `json:"ci_failure,omitempty"`
	// CIUnavailable says which: the CI Watcher not Watching, or no CI
	// Runs for the pushed commit (CI unavailable).
	CIUnavailable *CIUnavailableView `json:"ci_unavailable,omitempty"`
	// CIItem is the pushed commit's item: the Tickets waiting on it and
	// the failure options (CI failed, CI cancelled and CI unavailable).
	CIItem *CIItemView `json:"ci_item,omitempty"`
	// Blocker is the Cancelled Blocker (blocked by a Cancelled Ticket).
	Blocker *BlockerView `json:"blocker,omitempty"`
	// Link is the Ticket's Link, whatever the reason: the row's badge, and
	// the external item when closed externally.
	Link *LinkView `json:"link,omitempty"`
	// Requirement is set when the Ticket's Spec is a Requirement.
	Requirement *NeedsMeRequirement `json:"requirement,omitempty"`
	// Spec is the Spec a Map Ticket or Prototype is on, as its context,
	// set when its Spec isn't a Requirement (Spec 213).
	Spec *NeedsMeSpec `json:"spec,omitempty"`
	// Prototype is set on a Ticket labelled prototype, for its badge.
	Prototype bool `json:"prototype,omitempty"`
	// Retro is set on a Ticket labelled retro, for its badge.
	Retro bool `json:"retro,omitempty"`
	// CIResult is the Ticket's CI result, whatever the reason, or a CI
	// item's commit's, for the row's badge; left out for a Ticket never
	// pushed while CI Tracking was on.
	CIResult CIResult `json:"ci_result,omitempty"`
	// PullRequest is the Ticket's pull request, for its item's link; left
	// out until the Pull Request Watcher reports one.
	PullRequest *PullRequestView `json:"pull_request,omitempty"`
	// ClosedPullRequests are the Ticket's pull requests closed without
	// merging, each naming its Repository while the Project has several,
	// in landing order (Pull request closed).
	ClosedPullRequests []PullRequestView `json:"closed_pull_requests,omitempty"`
	// BaseDiverged is the local and remote base commits (Base diverged,
	// whose row has no Ticket: its ID is empty).
	BaseDiverged *BaseDivergedView `json:"base_diverged,omitempty"`
	// BaseMissing is the missing base's branch and the fix (Base missing,
	// whose row has no Ticket either).
	BaseMissing *BaseMissingView `json:"base_missing,omitempty"`
	// RepositoryMissing is the missing Repository's path, Clone URL,
	// problem and fix (Repository missing, whose row has no Ticket).
	RepositoryMissing *RepositoryMissingView `json:"repository_missing,omitempty"`
	// NotLanded is the Repositories the Ticket has landed in and those it
	// hasn't, with why (Accepted but not landed, while the Project has
	// several Repositories).
	NotLanded *NotLandedView `json:"not_landed,omitempty"`
}

// NeedsMeRequirement is the Requirement a Needs Me row's Ticket is on:
// its Spec ID and title, and the Ticket's Stage, set only on a Stage
// Ticket.
type NeedsMeRequirement struct {
	Spec  string           `json:"spec"`
	Title string           `json:"title"`
	Stage domain.StageName `json:"stage,omitempty"`
}

// NeedsMeSpec is the Spec a Needs Me row's Map Ticket or Prototype is on:
// its Spec ID and title, and whether it is a Map.
type NeedsMeSpec struct {
	Spec  string `json:"spec"`
	Title string `json:"title"`
	Map   bool   `json:"map,omitempty"`
}

// RoundSummary is a Round's number and how many of its Questions are open.
type RoundSummary struct {
	Number    int `json:"number"`
	Open      int `json:"open"`
	Questions int `json:"questions"`
}

// FailureSummary is the latest Failure Report's headline (the first line
// of what_broke) and how many reports the Ticket has.
type FailureSummary struct {
	Headline string `json:"headline"`
	Reports  int    `json:"reports"`
}

// NeedsMeReasonViews are the reasons in group order, each with what it
// means for the published workflow.
var NeedsMeReasonViews = []NeedsMeReasonView{
	{Reason: ReasonNeedsAnswer, Meaning: "Needs Answer: a Round waits for the human's answers"},
	{Reason: ReasonAnswered, Meaning: "Answered: answers wait for the asking session to pick them up"},
	{Reason: ReasonStopped, Meaning: "Stopped: the human stopped the Claim's session; resume, take over or release it"},
	{Reason: ReasonSilentClaim, Meaning: "Silent Claim: In Progress, no activity for 30 minutes"},
	{Reason: ReasonLongClaim, Meaning: "Long Claim: In Progress and active, with no claim or pick_up_answers for over 60 minutes"},
	{Reason: ReasonFailed, Meaning: "Failed: a Failure Report waits for the human"},
	{Reason: ReasonInReview, Meaning: "In Review: finished work waits to be accepted or rejected"},
	{Reason: ReasonCIFailed, Meaning: "CI failed: one item per pushed commit, listing the Tickets waiting on it; a failure option acts on all of them"},
	{Reason: ReasonCICancelled, Meaning: "CI cancelled: one item per pushed commit whose CI was cancelled"},
	{Reason: ReasonBaseDiverged, Meaning: "Base diverged: a Project's local base has diverged from the remote's; reconcile it by hand"},
	{Reason: ReasonBaseMissing, Meaning: "Base missing: a Project's base branch exists nowhere; create it or change it in Options"},
	{Reason: ReasonRepositoryMissing, Meaning: "Repository missing: the Factory has no clone of a Repository; clone it or set its local path"},
	{Reason: ReasonPullRequestClosed, Meaning: "Pull request closed: Accepted with its pull request closed without merging"},
	{Reason: ReasonToMerge, Meaning: "To merge: Accepted under Branch or Pull Request; merge it to the base"},
	{Reason: ReasonCIUnavailable, Meaning: "CI unavailable: one item per pushed commit with no word of its CI 10 minutes after its push"},
	{Reason: ReasonAcceptedNotLanded, Meaning: "Accepted but not landed: Accepted over 10 minutes and not in the base, unless its CI is running"},
	{Reason: ReasonBlockedByCancelled, Meaning: "Blocked by a Cancelled Ticket with no Replacements"},
	{Reason: ReasonNewIssue, Meaning: "New issue: an Incoming Issue awaiting triage"},
	{Reason: ReasonClosedExternally, Meaning: "Closed externally: its linked item was closed, deleted or left its Integration's query; cancel, mark Done or unlink it"},
}

// NeedsMeReasons are the reasons in group order, as get_workflow
// publishes them; store.NeedsMeReasons is the same list as strings.
var NeedsMeReasons = func() []NeedsMeReason {
	out := make([]NeedsMeReason, len(NeedsMeReasonViews))
	for i, r := range NeedsMeReasonViews {
		out[i] = r.Reason
	}
	return out
}()
