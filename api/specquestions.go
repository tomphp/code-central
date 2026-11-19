package api

import "github.com/tomoram/code-central/domain"

// SpecQuestions is get_spec_questions' result: the roll-up of a Spec's
// Questions. Questions sit only on Tickets, so it lists each of the Spec's
// Tickets that has Rounds, in claim order, with its Rounds.
type SpecQuestions struct {
	Spec    string            `json:"spec"`
	Tickets []TicketQuestions `json:"tickets"`
}

// TicketQuestions is one Ticket's Rounds in a Spec's roll-up.
type TicketQuestions struct {
	ID     string       `json:"id"`
	Title  string       `json:"title"`
	State  domain.State `json:"state"`
	Rounds []RoundView  `json:"rounds"`
}

// SpecInput names one Spec, for reads that need nothing else.
type SpecInput struct {
	Spec Ref `json:"spec" path:"n" desc:"The Spec." required:"true"`
}
