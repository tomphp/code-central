package api

import "github.com/tomoram/code-central/domain"

// ClaimInput is claim's input: narrow to a Spec or name a Ticket, not both.
type ClaimInput struct {
	Spec      Ref    `json:"spec,omitempty" desc:"Claim the next Ticket in this Spec."`
	Ticket    Ref    `json:"ticket,omitempty" desc:"Claim this Ticket."`
	SessionID string `json:"session_id,omitempty" desc:"Your session ID, shown to the human for resuming you."`
	// WorkingDir and ResumeCommand are free-form and never parsed; nil
	// leaves them unset.
	WorkingDir    *string `json:"working_dir,omitempty" desc:"The directory you work in, where the human runs the resume command. A hook usually fills it in for you."`
	ResumeCommand *string `json:"resume_command,omitempty" desc:"The command that resumes your session from working_dir, shown to the human as is. Without one the human is shown claude --resume <session_id>."`
}

// ClaimResult is claim's result: the Claim ID with the full Ticket view
// and the Stage Guide for its Stage (implementation when it has none),
// or {ticket: null} when nothing can be claimed.
type ClaimResult struct {
	ClaimID    string          `json:"claim_id,omitempty"`
	Ticket     *TicketView     `json:"ticket"`
	StageGuide *StageGuideView `json:"stage_guide,omitempty"`
	// ModelChoice is the model and effort the Claim's first turn runs
	// with, as far as Code Central sets them.
	ModelChoice *ModelChoice `json:"model_choice,omitempty"`
}

// FinishInput is finish's input.
type FinishInput struct {
	Ticket  Ref    `json:"ticket" path:"n" desc:"The Ticket." required:"true"`
	ClaimID string `json:"claim_id" desc:"The Claim ID claim returned." required:"true"`
	Summary string `json:"summary" desc:"What was done, where, and what the reviewer should check, in Markdown." required:"true"`
	// Branch and Commit name the work's branch and its head commit in the
	// Home Repository. Both are optional and free-form; one left out keeps
	// the stored value. A Ticket with a branch is Accepted, not Done, when
	// the human accepts it.
	Branch string `json:"branch,omitempty" desc:"The branch the work is on, if any. Accepting work on a branch moves it to Accepted, to land once merged."`
	Commit string `json:"commit,omitempty" desc:"The branch's head commit, if any."`
	// Repositories are the branch and head commit in each other
	// Repository the work changed (Spec code-central-848); the Home
	// Repository may be given here instead of in Branch and Commit, not
	// in both. The finish replaces the Ticket's Changed Repositories.
	Repositories []RepositoryWork `json:"repositories,omitempty" desc:"With several Repositories: each other Repository you committed in. branch and commit stay the Home Repository's."`
	// Recommendation is the Spec recommendation, implement or split:
	// required on a spec Stage and refused on any other Ticket. It
	// replaces the one an earlier finish gave.
	Recommendation domain.Recommendation `json:"recommendation,omitempty" desc:"Required on a spec Stage only: implement or split." enum:"implement,split"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}

// FinishResult is finish's result: the Acknowledgement and the next step,
// or with full the Ticket and the next step.
type FinishResult struct {
	*Acknowledgement
	Ticket   *TicketView `json:"ticket,omitempty"`
	NextStep string      `json:"next_step"`
}

// RejectInput is reject's input.
type RejectInput struct {
	Ticket  Ref    `json:"ticket" path:"n"`
	Comment string `json:"comment"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// LandInput is land's input: the commit that put the work into the base
// of one of the Ticket's Changed Repositories.
type LandInput struct {
	Ticket Ref    `json:"ticket" path:"n" desc:"The Ticket." required:"true" cli:"<id>"`
	Commit string `json:"commit" desc:"The commit that put the work into the base, such as the merge commit." required:"true" cli:"--commit <sha>"`
	// Repository is the Repository Name of the base the work landed in;
	// blank for the Home Repository.
	Repository string `json:"repository,omitempty" desc:"The Repository Name whose base it landed in; the Home Repository when left out." cli:"--repository <name>"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}
