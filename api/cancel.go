package api

// CancelInput is cancel's input. An agent must name at least one
// Replacement; the human may name none. The reason is stored as a comment.
type CancelInput struct {
	Ticket       Ref    `json:"ticket" path:"n" desc:"The Ticket to cancel." required:"true"`
	Replacements []Ref  `json:"replacements,omitempty" desc:"The Tickets taking over its work (at least one). IDs: <key>-<n> or <n>." required:"true"`
	Reason       string `json:"reason,omitempty" desc:"Why, stored as a comment."`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}
