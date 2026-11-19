package api

// ToMergeOption is a To merge option: what the human may choose on an
// Accepted Ticket they merge, an operation's name.
type ToMergeOption string

// ToMergeOptions are the To merge options, in the order they are offered.
func ToMergeOptions() []ToMergeOption {
	return []ToMergeOption{ToMergeSendBack, ToMergeNewFixTicket}
}

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
