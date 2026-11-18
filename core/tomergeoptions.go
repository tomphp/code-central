package core

import (
	"github.com/tomoram/code-central/domain"
)

// ToMergeOption is an option the human may choose on an Accepted Ticket
// they merge, as the Accepted panel offers it: an operation's name.
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

// toMergeOptions are the To merge options of t, from its Project's
// evaluation e: nil unless t is Accepted under a Landing Method the human
// merges (Branch or Pull Request) in one of its Repositories, whatever
// its Needs Me reason; then send_back where the human's send-back is
// accepted, and new_fix_ticket where it takes a Spec fix Ticket. They are
// worked out by the predicates those operations check, so the read and
// the write agree.
func toMergeOptions(e *evaluation, t domain.Ticket) []ToMergeOption {
	if t.State != domain.Accepted || !ticketLandingPolicies(e.p, t).Any(domain.LandingPolicy.LandsOnHumanMerge) {
		return nil
	}
	options := []ToMergeOption{}
	if sharedSendBackRefusal(e, t) == nil {
		options = append(options, ToMergeSendBack)
	}
	if takesSpecFixTicket(e, t) {
		options = append(options, ToMergeNewFixTicket)
	}
	return options
}
