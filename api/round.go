package api

import (
	"time"

	"github.com/tomoram/code-central/domain"
)

// QuestionInput is one Question of a Round being posted.
type QuestionInput struct {
	Title       string `json:"title" desc:"A short title." required:"true"`
	Body        string `json:"body" desc:"The Question in Markdown, with any choices as prose." required:"true"`
	Recommended string `json:"recommended,omitempty" desc:"Your recommended answer, if you have one."`
}

// PostRoundInput is post_round's input.
type PostRoundInput struct {
	Ticket    Ref             `json:"ticket" path:"n" desc:"The Ticket." required:"true"`
	ClaimID   string          `json:"claim_id" desc:"The Claim ID claim returned." required:"true"`
	SessionID string          `json:"session_id,omitempty" desc:"Your session ID, shown to the human for resuming you."`
	Questions []QuestionInput `json:"questions" minItems:"1" desc:"The Questions, each with a title, a body and an optional recommended answer." required:"true"`
	// Full asks for the Round number and the next step without the
	// Acknowledgement: post_round's record is its Round, not the Ticket.
	Full bool `json:"full,omitempty" desc:"Return only the Round number and the next step instead of the acknowledgement."`
}

// PostRoundResult is post_round's result: the Acknowledgement (none with
// full), the Round number and the next step.
type PostRoundResult struct {
	*Acknowledgement
	Round    int    `json:"round"`
	NextStep string `json:"next_step"`
}

// PickUpAnswersInput is pick_up_answers' input.
type PickUpAnswersInput struct {
	Ticket  Ref    `json:"ticket" path:"n" desc:"The Ticket." required:"true" cli:"<id>"`
	ClaimID string `json:"claim_id" desc:"The Claim ID claim returned." required:"true" cli:"--claim <claim-id>"`
}

// PickUpAnswersResult is the latest Round with every answer and
// dismissal, and the comments added since it was posted.
type PickUpAnswersResult struct {
	Ticket   string        `json:"ticket"`
	Round    RoundView     `json:"round"`
	Comments []CommentView `json:"comments"`
}

// AnswerInput is the human's answer(question, text). Questions are
// numbered across their Ticket.
type AnswerInput struct {
	Ticket   Ref    `json:"ticket" path:"n"`
	Question int    `json:"question"`
	Text     string `json:"text"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// DismissQuestionInput is the human's dismiss_question(question).
type DismissQuestionInput struct {
	Ticket   Ref `json:"ticket" path:"n"`
	Question int `json:"question"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// AcceptRecommendedInput is the human's accept_recommended(round).
type AcceptRecommendedInput struct {
	Ticket Ref `json:"ticket" path:"n"`
	Round  int `json:"round"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// RoundView is one Round as reads show it.
type RoundView struct {
	Number    int            `json:"number"`
	Claim     int            `json:"claim"`
	SessionID string         `json:"session_id,omitempty"`
	Posted    time.Time      `json:"posted"`
	Questions []QuestionView `json:"questions"`
}

// QuestionView is one Question: open, answered or dismissed.
type QuestionView struct {
	Number      int                   `json:"number"`
	Title       string                `json:"title"`
	Body        string                `json:"body"`
	Recommended string                `json:"recommended,omitempty"`
	Status      domain.QuestionStatus `json:"status"`
	Answer      string                `json:"answer,omitempty"`
	Closed      *time.Time            `json:"closed,omitempty"`
}
