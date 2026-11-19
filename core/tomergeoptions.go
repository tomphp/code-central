package core

import (
	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// toMergeOptions are the To merge options of t: nil unless t is Accepted
// under a Landing Method the human merges (Branch or Pull Request) in one
// of its Repositories, whatever its Needs Me reason, so a CI item's or
// Pull request closed has them too; none for a Spec-waiting Ticket that
// isn't its Spec's lead, whose Spec's item is the lead's; otherwise
// send_back where the human's send-back is accepted, and new_fix_ticket
// where it takes a Spec fix Ticket. They are worked out by the predicates
// those operations check, so an option offered is accepted.
func (e *evaluation) toMergeOptions(t domain.Ticket) []api.ToMergeOption {
	if t.State != domain.Accepted || !ticketLandingPolicies(e.p, t).Any(domain.LandingPolicy.LandsOnHumanMerge) {
		return nil
	}
	options := []api.ToMergeOption{}
	if lead, waiting := e.evaluated().specPullRequestLead(t); waiting && !lead {
		return options
	}
	if e.sharedSendBackRefusal(t) == nil {
		options = append(options, api.ToMergeSendBack)
	}
	if e.takesSpecFixTicket(t) {
		options = append(options, api.ToMergeNewFixTicket)
	}
	return options
}
