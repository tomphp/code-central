package api

import "github.com/tomoram/code-central/domain"

// BlockerInput is add_blocker's and remove_blocker's input: ticket waits
// on blocker.
type BlockerInput struct {
	Ticket  Ref `json:"ticket" path:"n" desc:"The Ticket that waits." required:"true" cli:"<id>"`
	Blocker Ref `json:"blocker" desc:"The Ticket it waits on." required:"true" cli:"<blocker-id>"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}

// BlockerView is one Blocker as get_ticket shows it, after following
// Replacements. Via names the Cancelled Blocker it stands in for, if any.
// A brief view shows a Done Blocker as its ID and State only
// (TicketView.Brief), leaving out its title, via and cleared.
type BlockerView struct {
	ID    string       `json:"id"`
	Title string       `json:"title,omitempty"`
	State domain.State `json:"state"`
	Via   string       `json:"via,omitempty"`
	// Cleared is whether it no longer blocks though it isn't Done, as one
	// on the same Spec does once it is merged into the Spec Branch. It is
	// always set, except on a brief view's Done Blocker.
	Cleared *bool `json:"cleared,omitempty"`
}
