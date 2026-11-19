package api

// NotLandedView is an Accepted but not landed item's Repositories, while
// the Project has several: those its Ticket has landed in, and those it
// hasn't, each with why, in landing order.
type NotLandedView struct {
	Landed    []string                  `json:"landed"`
	NotLanded []NotLandedRepositoryView `json:"not_landed"`
}

// NotLandedRepositoryView is a Changed Repository a Ticket hasn't landed
// in, and why.
type NotLandedRepositoryView struct {
	Name string `json:"name"`
	// Reason is why: its broken base, or what the Factory last said about
	// it since the Ticket was accepted; left out when there's none to
	// give.
	Reason string `json:"reason,omitempty"`
}
