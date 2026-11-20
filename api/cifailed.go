package api

// CIFailureOption is a failure option the human may choose on a CI item,
// named for its operation.
type CIFailureOption string

// CIFailureOptions are the failure options, in the order they are
// offered.
func CIFailureOptions() []CIFailureOption {
	return []CIFailureOption{OptionRerunCI, OptionLandAnyway, OptionSendBack, OptionNewFixTicket}
}

// CIFailureSummary is what failed on a CI failed item's pushed commit:
// the commit and its failed runs' workflow file names.
type CIFailureSummary struct {
	Commit    string   `json:"commit"`
	Workflows []string `json:"workflows"`
}

// CIResponseInput is land_anyway's and rerun_ci's input: the Ticket, and
// the Repository whose CI item to act on.
type CIResponseInput struct {
	Ticket Ref `json:"ticket" path:"n"`
	// Repository is the Repository Name of the CI item to act on; left
	// out, the Ticket's only CI item's.
	Repository string `json:"repository,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// NewFixTicketInput is new_fix_ticket's input: the CI failed Ticket and
// the note, which is required.
type NewFixTicketInput struct {
	Ticket Ref    `json:"ticket" path:"n"`
	Note   string `json:"note"`
	// Repository is the Repository Name of the CI item to fix; left out,
	// the Ticket's only CI item's.
	Repository string `json:"repository,omitempty"`
	// Full asks for the fix Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// The failure options.
const (
	// OptionRerunCI is rerun_ci, offered under both Push Landing Methods.
	OptionRerunCI CIFailureOption = "rerun_ci"
	// OptionLandAnyway is land_anyway, offered only under Merge + Push:
	// under Branch and Pull Request the human's merge decides.
	OptionLandAnyway CIFailureOption = "land_anyway"
	// OptionSendBack is the human's send_back from Accepted: the next
	// Claim fixes the work on its branch (code-central-264).
	OptionSendBack CIFailureOption = "send_back"
	// OptionNewFixTicket is new_fix_ticket: a Ready Ticket on the
	// CI Fixes Spec fixes it forward, and the failed Tickets keep
	// waiting for a later commit that passes (code-central-433).
	OptionNewFixTicket CIFailureOption = "new_fix_ticket"
)
