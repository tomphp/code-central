package api

import "time"

// ReportFailureInput is report_failure's input. Every field is required.
type ReportFailureInput struct {
	Ticket          Ref    `json:"ticket" path:"n" desc:"The Ticket." required:"true"`
	ClaimID         string `json:"claim_id" desc:"The Claim ID claim returned." required:"true"`
	WhatBroke       string `json:"what_broke" desc:"A headline plus the symptom, in Markdown." required:"true"`
	Why             string `json:"why" desc:"Why it broke, in Markdown." required:"true"`
	WhatToFix       string `json:"what_to_fix" desc:"What has to change before this Ticket can go on, in Markdown." required:"true"`
	WhereWorkStands string `json:"where_work_stands" desc:"Where the work stands: branch, commits, what is done, in Markdown." required:"true"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}

// GiveUpInput is give_up's input; the note is required.
type GiveUpInput struct {
	Ticket  Ref    `json:"ticket" path:"n" desc:"The Ticket." required:"true"`
	ClaimID string `json:"claim_id" desc:"The Claim ID claim returned." required:"true"`
	Note    string `json:"note" desc:"How far you got and why you are stopping, in Markdown." required:"true"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}

// SendBackInput is send_back's input. From Failed (the human's) the note
// is optional, and a send-back with no note means nothing has changed.
// From Accepted (the agent's on a conflict, ticket 72, or the human's to
// fix a CI failure, code-central-264) the note is required.
type SendBackInput struct {
	Ticket Ref    `json:"ticket" path:"n" desc:"The Ticket." required:"true"`
	Note   string `json:"note,omitempty" desc:"Why it can't land: the base commit and the conflicting files, in Markdown." required:"true"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}

// FailureReportView is one Failure Report as reads show it.
type FailureReportView struct {
	Number int `json:"number"`
	Claim  int `json:"claim"`
	// SessionID, WorkingDir and ResumeCommand are copies of the Claim's,
	// made when the report was written.
	SessionID       string    `json:"session_id,omitempty"`
	WorkingDir      string    `json:"working_dir,omitempty"`
	ResumeCommand   string    `json:"resume_command,omitempty"`
	Actor           string    `json:"actor"`
	Time            time.Time `json:"time"`
	WhatBroke       string    `json:"what_broke"`
	Why             string    `json:"why"`
	WhatToFix       string    `json:"what_to_fix"`
	WhereWorkStands string    `json:"where_work_stands"`
}

// FailFinishInput is the Factory's fail_finish(ticket, what_broke, why,
// what_to_fix, where_work_stands, full?): a Failure Report on a finish
// the finish check found wrong. Every field is required.
type FailFinishInput struct {
	Ticket          Ref    `json:"ticket" path:"n"`
	WhatBroke       string `json:"what_broke"`
	Why             string `json:"why"`
	WhatToFix       string `json:"what_to_fix"`
	WhereWorkStands string `json:"where_work_stands"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}
