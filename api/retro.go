package api

import "github.com/tomoram/code-central/domain"

// SubmitRetroInput is submit_retro's input: the finished work, a Ticket or
// a Spec, and the human's optional note.
type SubmitRetroInput struct {
	Target Ref    `json:"target" desc:"The finished Ticket or Spec the Retro looks back on." required:"true" cli:"<ticket|spec>"`
	Note   string `json:"note,omitempty" desc:"What the human wants the Retro to look at, in Markdown; optional." cli:"--note <md>"`
	// Full asks for the Ticket in place of its Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}

// RetroView is a Retro as a read of its target lists it.
type RetroView struct {
	ID    string       `json:"id"`
	Title string       `json:"title"`
	State domain.State `json:"state"`
}
