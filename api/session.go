package api

// SetSessionInput is set_session's input. The session ID is never
// checked; it is only shown to the human for resuming the session. The
// working directory and Resume Command are free-form and never parsed:
// one that isn't given (nil) leaves the stored value alone, and one given
// empty clears it.
type SetSessionInput struct {
	Ticket        Ref     `json:"ticket" path:"n" desc:"The Ticket." required:"true"`
	ClaimID       string  `json:"claim_id" desc:"The Claim ID claim returned." required:"true"`
	SessionID     string  `json:"session_id" desc:"Your session ID." required:"true"`
	WorkingDir    *string `json:"working_dir,omitempty" desc:"The directory you work in now. Left out, the recorded one stays."`
	ResumeCommand *string `json:"resume_command,omitempty" desc:"The command that resumes your session from working_dir. Left out, the recorded one stays."`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}

// ReleaseInput is release's input; the note is optional.
type ReleaseInput struct {
	Ticket Ref    `json:"ticket" path:"n"`
	Note   string `json:"note,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// StopInput is stop's input; the note is optional.
type StopInput struct {
	Ticket Ref    `json:"ticket" path:"n"`
	Note   string `json:"note,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// ResumeInput is resume's input; the note is optional.
type ResumeInput struct {
	Ticket Ref    `json:"ticket" path:"n"`
	Note   string `json:"note,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}
