package api

// SubmitMapInput is submit_map's input: the Map's free text, whose first
// line becomes the Spec's title.
type SubmitMapInput struct {
	Text string `json:"text" desc:"The Map's text, in Markdown; its first line becomes the Spec's title." required:"true"`
	// Draft saves every Ticket the submit writes as Draft, not Ready, so
	// nothing is claimed until the human moves them to Ready.
	Draft bool `json:"draft,omitempty" desc:"Save the Charting Ticket as Draft instead of Ready, so nothing is claimed until the human moves it to Ready."`
	// Full asks for the Spec and Charting Ticket in place of their
	// Acknowledgements.
	Full bool `json:"full,omitempty" desc:"Return the whole Spec and Charting Ticket instead of their acknowledgements."`
}

// SubmitMapResult is the new Map: its Spec and its Charting Ticket, each
// acknowledged, or with full each the record.
type SubmitMapResult struct {
	Spec     Written[SpecView]   `json:"spec"`
	Charting Written[TicketView] `json:"charting"`
}

// MapView is the Map block on a Spec read, present only when the Spec is
// labelled wayfinder:map. Nothing in it is stored.
type MapView struct {
	// Decided is the Map's Wayfinder Tickets that are Done.
	Decided int `json:"decided"`
	// Total is its Wayfinder Tickets that aren't Cancelled.
	Total int `json:"total"`
	// NeedsYou is its Tickets with a Needs Me reason, Prototypes included.
	NeedsYou int `json:"needs_you"`
	// Guided is whether its guided Tag is on: the Factory then starts its
	// next decision whenever it is idle.
	Guided bool `json:"guided"`
	// CanWriteSpec is whether it has no unfinished Tickets.
	CanWriteSpec bool `json:"can_write_spec"`
	// Requirements are the Specs whose map Tag names this Map: the
	// Requirements Write the Spec made from it, in creation order.
	Requirements []string `json:"requirements"`
}
