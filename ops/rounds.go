package ops

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
)

// Rounds of Questions, and Failure Reports and giving up.

// PostRound is post_round. Question numbers run across their Ticket, so
// the path names the Ticket and the body the Questions.
var PostRound = &Definition[api.PostRoundInput, api.PostRoundResult]{
	Name: "post_round", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/post_round",
	Doc:  "Ask the human a Round of Questions on a Ticket you hold, in one write; it moves to Needs Answer and you keep the Claim. Returns an acknowledgement with the Round number and the next step, or only those with full.",
	Call: (*core.Core).PostRound,
}

// PickUpAnswers is pick_up_answers.
var PickUpAnswers = &Definition[api.PickUpAnswersInput, api.PickUpAnswersResult]{
	Name: "pick_up_answers", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/pick_up_answers",
	Doc:  "Pick up the answers to your latest Round on an Answered Ticket you hold, moving it back to In Progress; returns the Round and comments since.",
	Call: (*core.Core).PickUpAnswers,
}

// Answer is the human's answer to one Question.
var Answer = &Definition[api.AnswerInput, api.Written[api.TicketView]]{
	Name: "answer", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/answer",
	Doc:  "Answer one Question of a Ticket's Rounds, as the human.",
	Call: (*core.Core).Answer,
}

// AcceptRecommended is the human's accept_recommended.
var AcceptRecommended = &Definition[api.AcceptRecommendedInput, api.Written[api.TicketView]]{
	Name: "accept_recommended", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/accept_recommended",
	Doc:  "Answer every unanswered Question of a Round with its recommended answer, as the human.",
	Call: (*core.Core).AcceptRecommended,
}

// DismissQuestion is the human's dismiss_question.
var DismissQuestion = &Definition[api.DismissQuestionInput, api.Written[api.TicketView]]{
	Name: "dismiss_question", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/dismiss_question",
	Doc:  "Dismiss one Question of a Ticket's Rounds without an answer, as the human.",
	Call: (*core.Core).DismissQuestion,
}

// GetSpecQuestions is the roll-up of a Spec's Questions, HTTP-only, for
// the UI.
var GetSpecQuestions = &Definition[api.SpecInput, api.SpecQuestions]{
	Name: "get_spec_questions", Audience: HumanOnly,
	HTTP: "GET /api/p/{key}/specs/{n}/questions",
	Doc:  "Get the Rounds of every Ticket of a Spec, rolled up.",
	Call: func(c *core.Core, ctx context.Context, key string, in api.SpecInput) (api.SpecQuestions, error) {
		return c.GetSpecQuestions(ctx, key, in.Spec)
	},
}

// ReportFailure is report_failure.
var ReportFailure = &Definition[api.ReportFailureInput, api.FinishResult]{
	Name: "report_failure", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/report_failure",
	Doc:  "Report a failure when nothing the human can answer will unblock you: what broke, why, what to fix and where the work stands. Ends your Claim; then stop working this Ticket. Returns an acknowledgement with the next step, or the Ticket with full.",
	Call: (*core.Core).ReportFailure,
}

// GiveUp is give_up.
var GiveUp = &Definition[api.GiveUpInput, api.FinishResult]{
	Name: "give_up", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/give_up",
	Doc:  "Give a claimed Ticket back to Ready when nothing is wrong but you can't finish it, with a note on how far you got. Ends your Claim; then stop working this Ticket. Returns an acknowledgement with the next step, or the Ticket with full.",
	Call: (*core.Core).GiveUp,
}

// SendBack is send_back: the agent's from Accepted on a conflict (ticket
// 72), or, with Actor set to human, the human's from Failed.
var SendBack = &Definition[api.SendBackInput, api.Written[api.TicketView]]{
	Name: "send_back", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/send_back",
	Doc:  "Send an Accepted Ticket back to Ready when its branch can't land, such as on a merge conflict, with a note naming the base commit and the conflicting files. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
	Call: (*core.Core).SendBack,
}

// ToDraft is the human's to_draft.
var ToDraft = &Definition[api.TicketInput, api.Written[api.TicketView]]{
	Name: "to_draft", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/to_draft",
	Doc:  "Move a Ticket back to Draft, as the human.",
	Call: (*core.Core).ToDraft,
}
