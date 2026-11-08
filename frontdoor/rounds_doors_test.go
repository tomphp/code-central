package frontdoor_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// askingTicket claims acme-3 as claim-1 and posts Round 1 with Question 1,
// recommended "SQLite".
func askingTicket(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	claimedTicket(t, srv)
	must(t, func() error {
		_, err := srv.Core.PostRound(context.Background(), "acme", core.PostRoundInput{Ticket: "3", ClaimID: "claim-1",
			Questions: []core.QuestionInput{{Title: "DB?", Body: "Which one?", Recommended: "SQLite"}}})
		return err
	})
}

// answeredTicket is askingTicket with Question 1 answered.
func answeredTicket(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	askingTicket(t, srv)
	must(t, func() error {
		_, err := srv.Core.Answer(core.WithActor(context.Background(), core.Human), "acme", core.AnswerInput{Ticket: "3", Question: 1, Text: "SQLite"})
		return err
	})
}

// Rounds and Questions (ticket 29).
func init() {
	answered := field("state", "Answered")
	for name, c := range map[string]opCase{
		"post_round": {
			setup: claimedTicket,
			ok: call{
				mcp:  map[string]any{"ticket": "acme-3", "claim_id": "claim-1", "questions": []map[string]any{{"title": "DB?", "body": "Which one?", "recommended": "SQLite"}}},
				http: httpCall{"POST", "/api/p/acme/tickets/3/post_round", `{"claim_id":"claim-1","questions":[{"title":"DB?","body":"Which one?","recommended":"SQLite"}]}`},
				cli:  []string{"acme-3", "--claim", "claim-1", "--questions", `[{"title":"DB?","body":"Which one?","recommended":"SQLite"}]`},
			},
			check: func(t *testing.T, out map[string]any) {
				if out["round"] != 1.0 {
					t.Fatalf("round %v in %v", out["round"], out)
				}
				field("next_step", "End your turn now, after running `code-central wait acme-3` in the background if you can run background commands.")(t, out)
			},
			bad: call{
				mcp:  map[string]any{"ticket": "acme-3", "claim_id": "claim-1", "questions": []map[string]any{}},
				http: httpCall{"POST", "/api/p/acme/tickets/3/post_round", `{"claim_id":"claim-1","questions":[]}`},
				cli:  []string{"acme-3", "--claim", "claim-1", "--questions", `[]`},
			},
			code: core.Invalid,
		},
		"pick_up_answers": {
			setup: answeredTicket,
			ok: call{
				mcp:  map[string]any{"ticket": 3, "claim_id": "claim-1"},
				http: httpCall{"POST", "/api/p/acme/tickets/3/pick_up_answers", `{"claim_id":"claim-1"}`},
				cli:  []string{"3", "--claim", "claim-1"},
			},
			check: func(t *testing.T, out map[string]any) {
				field("ticket", "acme-3")(t, out)
				qs := out["round"].(map[string]any)["questions"].([]any)
				field("answer", "SQLite")(t, qs[0].(map[string]any))
			},
			bad: call{
				mcp:  map[string]any{"ticket": 3, "claim_id": "claim-9"},
				http: httpCall{"POST", "/api/p/acme/tickets/3/pick_up_answers", `{"claim_id":"claim-9"}`},
				cli:  []string{"3", "--claim", "claim-9"},
			},
			code: core.StaleClaim,
		},
		"answer": {
			setup: askingTicket,
			ok:    call{http: httpCall{"POST", "/api/p/acme/tickets/3/answer", `{"question":1,"text":"Postgres"}`}},
			check: ticketAcknowledgement("acme-3", "Answered", "state", "rounds", "moves", "needs_me_reason"),
			full:  answered,
			bad:   call{http: httpCall{"POST", "/api/p/acme/tickets/3/answer", `{"question":1,"text":""}`}},
			code:  core.Invalid,
		},
		"accept_recommended": {
			setup: askingTicket,
			ok:    call{http: httpCall{"POST", "/api/p/acme/tickets/3/accept_recommended", `{"round":1}`}},
			check: ticketAcknowledgement("acme-3", "Answered", "state", "rounds", "moves", "needs_me_reason"),
			full:  answered,
			bad:   call{http: httpCall{"POST", "/api/p/acme/tickets/3/accept_recommended", `{"round":2}`}},
			code:  core.NotFound,
		},
		"dismiss_question": {
			setup: askingTicket,
			ok:    call{http: httpCall{"POST", "/api/p/acme/tickets/3/dismiss_question", `{"question":1}`}},
			check: ticketAcknowledgement("acme-3", "Answered", "state", "rounds", "moves", "needs_me_reason"),
			full:  answered,
			bad:   call{http: httpCall{"POST", "/api/p/acme/tickets/3/dismiss_question", `{"question":9}`}},
			code:  core.NotFound,
		},
		"get_spec_questions": {
			setup: askingTicket,
			ok:    call{http: httpCall{"GET", "/api/p/acme/specs/1/questions", ""}},
			check: func(t *testing.T, out map[string]any) {
				field("spec", "acme-1")(t, out)
				tickets, _ := out["tickets"].([]any)
				if len(tickets) != 1 {
					t.Fatalf("tickets %v", out["tickets"])
				}
				field("id", "acme-3")(t, tickets[0].(map[string]any))
			},
			bad:  call{http: httpCall{"GET", "/api/p/acme/specs/9/questions", ""}},
			code: core.NotFound,
		},
	} {
		cases[name] = c
	}
}
