package api

// NewTicketInput is new_ticket's input: the Spec and the free text, whose
// first line becomes the Ticket's title.
type NewTicketInput struct {
	Spec Ref    `json:"spec" desc:"The Spec to open the Ticket on." required:"true"`
	Text string `json:"text" desc:"The work to do, in Markdown; its first line becomes the Ticket's title." required:"true"`
	// Draft saves the Ticket as Draft, not Ready, so nothing is claimed
	// until the human moves it to Ready.
	Draft bool `json:"draft,omitempty" desc:"Save the Ticket as Draft instead of Ready, so nothing is claimed until the human moves it to Ready."`
	// Full asks for the Ticket in place of its Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}
