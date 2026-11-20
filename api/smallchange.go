package api

// SubmitSmallChangeInput is submit_small_change's input: the free text,
// whose first line becomes the Ticket's title.
type SubmitSmallChangeInput struct {
	Text string `json:"text" desc:"The change to make, in Markdown; its first line becomes the Ticket's title." required:"true"`
	// Draft saves every Ticket the submit writes as Draft, not Ready, so
	// nothing is claimed until the human moves them to Ready.
	Draft bool `json:"draft,omitempty" desc:"Save the Ticket as Draft instead of Ready, so nothing is claimed until the human moves it to Ready."`
	// Full asks for the Ticket in place of its Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}
