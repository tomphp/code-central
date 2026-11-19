package api

// SubmitPrototypeInput is submit_prototype's input: the Spec it is on and
// the Prototype's free text, whose first line becomes its title.
type SubmitPrototypeInput struct {
	Spec Ref    `json:"spec" desc:"The Spec the Prototype is on." required:"true"`
	Text string `json:"text" desc:"What to prototype and the question it answers, in Markdown; its first line becomes the title." required:"true"`
	// Full asks for the Ticket in place of its Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}
