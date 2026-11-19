package api

// NeedsMeReason is why a Ticket waits on the human. "" means it doesn't.
type NeedsMeReason string

// The reasons, in group order: cheapest to unblock first.
const (
	ReasonNeedsAnswer NeedsMeReason = "needs_answer"
	ReasonAnswered    NeedsMeReason = "answered"
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
