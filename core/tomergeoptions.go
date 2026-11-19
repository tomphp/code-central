package core

import (
	"github.com/tomoram/code-central/domain"
)

// ToMergeOption is a To merge option: what the human may choose on an
// Accepted Ticket they merge, an operation's name.
type ToMergeOption string

const (
	// ToMergeSendBack is the human's send_back from Accepted, so the next
	// Claim fixes the work on its branch: under Branch, and under Pull
	// Request without Land per Spec.
	ToMergeSendBack ToMergeOption = "send_back"
	// ToMergeNewFixTicket is new_fix_ticket on a Spec's pull-request lead
	// under Pull Request + Land per Spec: a Ready Ticket on the Spec, whose
	// accept is pushed to the Spec's pull request.
	ToMergeNewFixTicket ToMergeOption = "new_fix_ticket"
)

// ToMergeOptions are the To merge options, in the order they are offered.
func ToMergeOptions() []ToMergeOption {
	return []ToMergeOption{ToMergeSendBack, ToMergeNewFixTicket}
}

// toMergeOptions are the To merge options of t: nil unless t is Accepted
// under a Landing Method the human merges (Branch or Pull Request) in one
// of its Repositories, whatever its Needs Me reason, so a CI item's or
// Pull request closed has them too; none for a Spec-waiting Ticket that
// isn't its Spec's lead, whose Spec's item is the lead's; otherwise
// send_back where the human's send-back is accepted, and new_fix_ticket
// where it takes a Spec fix Ticket. They are worked out by the predicates
// those operations check, so an option offered is accepted.
func (e *evaluation) toMergeOptions(t domain.Ticket) []ToMergeOption {
	if t.State != domain.Accepted || !ticketLandingPolicies(e.p, t).Any(domain.LandingPolicy.LandsOnHumanMerge) {
		return nil
	}
	options := []ToMergeOption{}
	if lead, waiting := e.evaluated().specPullRequestLead(t); waiting && !lead {
		return options
	}
	if e.sharedSendBackRefusal(t) == nil {
		options = append(options, ToMergeSendBack)
	}
	if e.takesSpecFixTicket(t) {
		options = append(options, ToMergeNewFixTicket)
	}
	return options
}
