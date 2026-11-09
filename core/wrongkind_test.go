package core_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// wrongKind is Spec acme-1 with Ticket acme-2 on it, and nothing as acme-3.
func (f fixture) wrongKind(t *testing.T) {
	t.Helper()
	f.work(t, store.Ready)
}

// wantNotFound checks err is not_found, with no field, saying msg.
func wantNotFound(t *testing.T, err error, msg string) {
	t.Helper()
	wantMessage(t, err, core.NotFound, "", msg)
}

// wantInvalid checks err is invalid with field, saying msg.
func wantInvalid(t *testing.T, err error, field, msg string) {
	t.Helper()
	wantMessage(t, err, core.Invalid, field, msg)
}

func wantMessage(t *testing.T, err error, code core.Code, field, msg string) {
	t.Helper()
	wantCode(t, err, code, field)
	if got := err.(*core.Error).Message; got != msg {
		t.Fatalf("message %q, want %q", got, msg)
	}
}

// ticketOps runs each operation whose main target is a Ticket on ref,
// with every other input valid, so only the lookup can fail.
func ticketOps(f fixture) map[string]func(core.Ref) error {
	c := f.core
	title := "T"
	q := []core.QuestionInput{{Title: "Q", Body: "B"}}
	return map[string]func(core.Ref) error{
		"get_ticket": func(r core.Ref) error { _, err := c.GetTicket(ctx, "acme", whole(r)); return err },
		"edit_ticket": func(r core.Ref) error {
			_, err := c.EditTicket(ctx, "acme", core.EditTicketInput{Ticket: r, Title: &title})
			return err
		},
		"update_linked_ticket": func(r core.Ref) error {
			_, err := c.UpdateLinkedTicket(integration, "acme", core.UpdateLinkedTicketInput{Ticket: r, Title: &title})
			return err
		},
		"claim": func(r core.Ref) error { _, err := c.Claim(ctx, "acme", core.ClaimInput{Ticket: r}); return err },
		"finish": func(r core.Ref) error {
			_, err := c.Finish(ctx, "acme", core.FinishInput{Ticket: r, ClaimID: "x", Summary: "s"})
			return err
		},
		"accept": func(r core.Ref) error { _, err := c.Accept(human, "acme", core.TicketInput{Ticket: r}); return err },
		"reject": func(r core.Ref) error {
			_, err := c.Reject(human, "acme", core.RejectInput{Ticket: r, Comment: "c"})
			return err
		},
		"comment": func(r core.Ref) error {
			_, err := c.Comment(ctx, "acme", core.CommentInput{Ticket: r, Body: "b"})
			return err
		},
		"approve": func(r core.Ref) error { _, err := c.Approve(ctx, "acme", core.TicketInput{Ticket: r}); return err },
		"defer": func(r core.Ref) error {
			_, err := c.Defer(ctx, "acme", core.DeferInput{Ticket: r, Reason: "r"})
			return err
		},
		"cancel": func(r core.Ref) error {
			_, err := c.Cancel(human, "acme", core.CancelInput{Ticket: r})
			return err
		},
		"set_session": func(r core.Ref) error {
			_, err := c.SetSession(ctx, "acme", core.SetSessionInput{Ticket: r, ClaimID: "x", SessionID: "s"})
			return err
		},
		"release": func(r core.Ref) error { _, err := c.Release(human, "acme", core.ReleaseInput{Ticket: r}); return err },
		"add_blocker": func(r core.Ref) error {
			_, err := c.AddBlocker(ctx, "acme", core.BlockerInput{Ticket: r, Blocker: "2"})
			return err
		},
		"remove_blocker": func(r core.Ref) error {
			_, err := c.RemoveBlocker(ctx, "acme", core.BlockerInput{Ticket: r, Blocker: "2"})
			return err
		},
		"report_failure": func(r core.Ref) error {
			_, err := c.ReportFailure(ctx, "acme", core.ReportFailureInput{Ticket: r, ClaimID: "x", WhatBroke: "a", Why: "b", WhatToFix: "c", WhereWorkStands: "d"})
			return err
		},
		"give_up": func(r core.Ref) error {
			_, err := c.GiveUp(ctx, "acme", core.GiveUpInput{Ticket: r, ClaimID: "x", Note: "n"})
			return err
		},
		"send_back": func(r core.Ref) error { _, err := c.SendBack(human, "acme", core.SendBackInput{Ticket: r}); return err },
		"to_draft":  func(r core.Ref) error { _, err := c.ToDraft(human, "acme", core.TicketInput{Ticket: r}); return err },
		"post_round": func(r core.Ref) error {
			_, err := c.PostRound(ctx, "acme", core.PostRoundInput{Ticket: r, ClaimID: "x", Questions: q})
			return err
		},
		"pick_up_answers": func(r core.Ref) error {
			_, err := c.PickUpAnswers(ctx, "acme", core.PickUpAnswersInput{Ticket: r, ClaimID: "x"})
			return err
		},
		"answer": func(r core.Ref) error {
			_, err := c.Answer(human, "acme", core.AnswerInput{Ticket: r, Question: 1, Text: "a"})
			return err
		},
		"accept_recommended": func(r core.Ref) error {
			_, err := c.AcceptRecommended(human, "acme", core.AcceptRecommendedInput{Ticket: r, Round: 1})
			return err
		},
		"dismiss_question": func(r core.Ref) error {
			_, err := c.DismissQuestion(human, "acme", core.DismissQuestionInput{Ticket: r, Question: 1})
			return err
		},
		"move": func(r core.Ref) error {
			_, err := c.Move(human, "acme", core.MoveInput{Ticket: r, ToState: store.Done})
			return err
		},
		"land": func(r core.Ref) error {
			_, err := c.Land(ctx, "acme", core.LandInput{Ticket: r, Commit: "abc"})
			return err
		},
		"transcript": func(r core.Ref) error { _, err := c.Transcript(ctx, "acme", r, 1); return err },
		"follow_transcript": func(r core.Ref) error {
			feed, err := c.FollowTranscript(ctx, "acme", r, 1, 0)
			if feed != nil {
				feed.Stop()
			}
			return err
		},
	}
}

func TestASpecGivenAsATicketNamesTheOperationToUseInstead(t *testing.T) {
	f := newFixture(t)
	f.wrongKind(t)
	special := map[string]string{
		"get_ticket":           "acme-1 is a Spec; use get_spec",
		"edit_ticket":          "acme-1 is a Spec; use edit_spec",
		"update_linked_ticket": "acme-1 is a Spec; use update_linked_spec",
		"claim":                "acme-1 is a Spec; pass it as spec to claim its next Ticket",
	}
	for op, run := range ticketOps(f) {
		t.Run(op, func(t *testing.T) {
			want, ok := special[op]
			if !ok {
				want = "acme-1 is a Spec, not a Ticket; get_spec lists its Tickets"
			}
			wantNotFound(t, run("acme-1"), want)
		})
	}
}

func TestAnIDOfNeitherKindIsNoTicket(t *testing.T) {
	f := newFixture(t)
	f.wrongKind(t)
	for op, run := range ticketOps(f) {
		t.Run(op, func(t *testing.T) {
			wantNotFound(t, run("acme-3"), "there is no Ticket acme-3")
		})
	}
}

// specOps runs each operation whose main target is a Spec on ref.
func specOps(f fixture) map[string]func(core.Ref) error {
	c := f.core
	title := "S"
	return map[string]func(core.Ref) error{
		"get_spec": func(r core.Ref) error { _, err := c.GetSpec(ctx, "acme", core.GetSpecInput{Spec: r}); return err },
		"edit_spec": func(r core.Ref) error {
			_, err := c.EditSpec(ctx, "acme", core.EditSpecInput{Spec: r, Title: &title})
			return err
		},
		"get_spec_questions": func(r core.Ref) error { _, err := c.GetSpecQuestions(human, "acme", r); return err },
		"update_linked_spec": func(r core.Ref) error {
			_, err := c.UpdateLinkedSpec(integration, "acme", core.UpdateLinkedSpecInput{Spec: r, Title: &title})
			return err
		},
	}
}

func TestATicketGivenAsASpecNamesItsSpecAndTheOperationToUseInstead(t *testing.T) {
	f := newFixture(t)
	f.wrongKind(t)
	want := map[string]string{
		"get_spec":           "acme-2 is a Ticket of Spec acme-1; use get_ticket",
		"edit_spec":          "acme-2 is a Ticket of Spec acme-1; use edit_ticket",
		"get_spec_questions": "acme-2 is a Ticket of Spec acme-1; use get_ticket",
		"update_linked_spec": "acme-2 is a Ticket of Spec acme-1; use update_linked_ticket",
	}
	for op, run := range specOps(f) {
		t.Run(op, func(t *testing.T) {
			wantNotFound(t, run("acme-2"), want[op])
		})
	}
}

func TestAnIDOfNeitherKindIsNoSpec(t *testing.T) {
	f := newFixture(t)
	f.wrongKind(t)
	for op, run := range specOps(f) {
		t.Run(op, func(t *testing.T) {
			wantNotFound(t, run("acme-3"), "there is no Spec acme-3")
		})
	}
}

func TestOperationsOnEitherKindAreUnchanged(t *testing.T) {
	f := newFixture(t)
	f.wrongKind(t)
	for _, ref := range []core.Ref{"acme-1", "acme-2"} {
		if _, err := f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: ref, Labels: []string{"x"}}); err != nil {
			t.Fatalf("add_labels %s: %v", ref, err)
		}
	}
	_, err := f.core.AddLabels(ctx, "acme", core.LabelsInput{Target: "acme-3", Labels: []string{"x"}})
	wantNotFound(t, err, "there is no Spec or Ticket acme-3")
}

// nestedOp runs an operation with ref in a nested field, and every other
// input valid, so only that lookup can fail.
type nestedOp struct {
	field   string
	run     func(core.Ref) error
	neither string // the message for an ID of neither kind; "" when it isn't an error
}

// nestedOps is each operation with a nested Ref, run on f.linked: Spec
// acme-1 with linked Ticket acme-2 on it, and nothing as acme-3.
func nestedOps(f fixture) map[string]nestedOp {
	c := f.core
	noSpec := "there is no Spec acme-3"
	noTicket := "there is no Ticket acme-3"
	return map[string]nestedOp{
		"create_ticket spec": {"spec", func(r core.Ref) error {
			_, err := c.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: r, Title: "T"})
			return err
		}, noSpec},
		"create_linked_ticket spec": {"spec", func(r core.Ref) error {
			_, err := c.CreateLinkedTicket(integration, "acme", core.CreateLinkedTicketInput{Spec: r, Title: "T", Link: ghLink("20")})
			return err
		}, noSpec},
		"claim spec": {"spec", func(r core.Ref) error { _, err := c.Claim(ctx, "acme", core.ClaimInput{Spec: r}); return err }, noSpec},
		"find_tickets spec": {"spec", func(r core.Ref) error {
			_, err := c.FindTickets(ctx, "acme", core.FindTicketsInput{Spec: r})
			return err
		}, ""},
		"update_linked_ticket spec": {"spec", func(r core.Ref) error {
			_, err := c.UpdateLinkedTicket(integration, "acme", core.UpdateLinkedTicketInput{Ticket: "2", Spec: &r})
			return err
		}, noSpec},
		"create_ticket blocked_by": {"blocked_by", func(r core.Ref) error {
			_, err := c.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "T", BlockedBy: []core.Ref{r}})
			return err
		}, noTicket},
		"add_blocker blocker": {"blocker", func(r core.Ref) error {
			_, err := c.AddBlocker(ctx, "acme", core.BlockerInput{Ticket: "2", Blocker: r})
			return err
		}, noTicket},
		"remove_blocker blocker": {"blocker", func(r core.Ref) error {
			_, err := c.RemoveBlocker(ctx, "acme", core.BlockerInput{Ticket: "2", Blocker: r})
			return err
		}, "acme-3 is not a Blocker of acme-2"},
		"cancel replacements": {"replacements", func(r core.Ref) error {
			_, err := c.Cancel(human, "acme", core.CancelInput{Ticket: "2", Replacements: []core.Ref{r}})
			return err
		}, noTicket},
	}
}

func TestAWrongKindIDInANestedFieldNamesTheFieldAndWhatToGiveInstead(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	for name, op := range nestedOps(f) {
		t.Run(name, func(t *testing.T) {
			if op.field == "spec" {
				wantInvalid(t, op.run("acme-2"), "spec", "acme-2 is a Ticket of Spec acme-1; pass acme-1 as spec")
				return
			}
			wantInvalid(t, op.run("acme-1"), op.field, "acme-1 is a Spec, not a Ticket; give a Ticket of it (get_spec lists them)")
		})
	}
}

func TestAnIDOfNeitherKindInANestedFieldKeepsItsMessage(t *testing.T) {
	f := newFixture(t)
	f.linked(t)
	for name, op := range nestedOps(f) {
		t.Run(name, func(t *testing.T) {
			err := op.run("acme-3")
			if op.neither == "" {
				if err != nil {
					t.Fatalf("err = %v, want none", err)
				}
				return
			}
			wantInvalid(t, err, op.field, op.neither)
		})
	}
}
