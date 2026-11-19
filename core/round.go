package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// postRoundNextStep is post_round's next step (its row in Workflow);
// <id> becomes the Ticket ID.
const postRoundNextStep = "End your turn now, after running `code-central wait <id>` in the background if you can run background commands."

// PostRound is post_round(ticket, claim_id, session_id?, questions): the
// holder posts a numbered Round in one write, moving In Progress to Needs
// Answer and keeping the Claim. The session ID defaults to the Claim's.
func (c *Core) PostRound(ctx context.Context, key string, in api.PostRoundInput) (api.PostRoundResult, error) {
	if len(in.Questions) == 0 {
		return api.PostRoundResult{}, invalid("questions", "a Round needs at least one Question")
	}
	for i, q := range in.Questions {
		if strings.TrimSpace(q.Title) == "" {
			return api.PostRoundResult{}, invalid(fmt.Sprintf("questions[%d].title", i), "every Question needs a title")
		}
		if strings.TrimSpace(q.Body) == "" {
			return api.PostRoundResult{}, invalid(fmt.Sprintf("questions[%d].body", i), "every Question needs a body")
		}
	}
	var res api.PostRoundResult
	v, err := c.updateTicket(key, "post_round", in.Ticket, in.Full, func(t *domain.Ticket) (bool, error) {
		claim, _ := claimToWrite(t)
		now := c.now()
		if err := transition(ctx, t, key, "post_round", in.ClaimID, now); err != nil {
			return false, err
		}
		// A given session ID also becomes the Claim's, so pings and the
		// resume command follow the session that asked.
		if in.SessionID != "" {
			claim.SessionID = in.SessionID
		}
		session := claim.SessionID
		r := domain.Round{Number: len(t.Rounds) + 1, Claim: claim.Number, SessionID: session, Posted: now}
		n := questionCount(*t)
		for _, q := range in.Questions {
			n++
			r.Questions = append(r.Questions, domain.Question{Number: n, Title: q.Title, Body: q.Body, Recommended: q.Recommended, Status: domain.QuestionOpen})
		}
		t.Rounds = append(t.Rounds, r)
		res = api.PostRoundResult{Round: r.Number, NextStep: strings.ReplaceAll(nextStep("post_round"), "<id>", DisplayID(key, t.Number))}
		return true, nil
	})
	if err != nil {
		return api.PostRoundResult{}, err
	}
	if v.Record == nil {
		res.Acknowledgement = &v.Acknowledgement
	}
	return res, nil
}

// PickUpAnswers is pick_up_answers(ticket, claim_id): the holder moves
// Answered to In Progress in one step and gets the latest Round back.
func (c *Core) PickUpAnswers(ctx context.Context, key string, in api.PickUpAnswersInput) (api.PickUpAnswersResult, error) {
	var res api.PickUpAnswersResult
	// With full, as the Ticket isn't returned: no before view is built.
	_, err := c.updateTicket(key, "pick_up_answers", in.Ticket, true, func(t *domain.Ticket) (bool, error) {
		if err := transition(ctx, t, key, "pick_up_answers", in.ClaimID, c.now()); err != nil {
			return false, err
		}
		r := t.Rounds[len(t.Rounds)-1]
		res = api.PickUpAnswersResult{Ticket: DisplayID(key, t.Number), Round: roundView(r), Comments: []api.CommentView{}}
		for _, cv := range commentViews(t.Comments) {
			if !cv.Time.Before(r.Posted) {
				res.Comments = append(res.Comments, cv)
			}
		}
		return true, nil
	})
	return res, err
}

// Answer is the human's answer(question, text). An answer can be changed
// until its Round is complete; the last open Question answered moves the
// Ticket to Answered.
func (c *Core) Answer(ctx context.Context, key string, in api.AnswerInput) (api.Written[api.TicketView], error) {
	if strings.TrimSpace(in.Text) == "" {
		return api.Written[api.TicketView]{}, invalid("text", "an answer needs text; to decline a Question, dismiss it")
	}
	return c.onQuestion(ctx, key, in.Ticket, in.Full, "answer", in.Question, func(q *domain.Question, now time.Time) {
		q.Status, q.Answer, q.Closed = domain.QuestionAnswered, in.Text, now
	})
}

// DismissQuestion is the human's dismiss_question(question): one Question
// declined without moving the Ticket, unless it was the last open one.
func (c *Core) DismissQuestion(ctx context.Context, key string, in api.DismissQuestionInput) (api.Written[api.TicketView], error) {
	return c.onQuestion(ctx, key, in.Ticket, in.Full, "dismiss_question", in.Question, func(q *domain.Question, now time.Time) {
		q.Status, q.Answer, q.Closed = domain.QuestionDismissed, "", now
	})
}

// AcceptRecommended is the human's accept_recommended(round): every open
// Question with a recommendation is answered with it.
func (c *Core) AcceptRecommended(ctx context.Context, key string, in api.AcceptRecommendedInput) (api.Written[api.TicketView], error) {
	find := func(t *domain.Ticket) (*domain.Round, error) {
		if in.Round < 1 || in.Round > len(t.Rounds) {
			return nil, notFound("there is no Round %d on %s", in.Round, DisplayID(key, t.Number))
		}
		return &t.Rounds[in.Round-1], nil
	}
	return c.onRound(ctx, key, in.Ticket, in.Full, "accept_recommended", find, func(r *domain.Round, now time.Time) bool {
		changed := false
		for i := range r.Questions {
			if q := &r.Questions[i]; q.Status == domain.QuestionOpen && q.Recommended != "" {
				q.Status, q.Answer, q.Closed = domain.QuestionAnswered, q.Recommended, now
				changed = true
			}
		}
		return changed
	})
}

// onQuestion runs a Round action on Question n of the Ticket.
func (c *Core) onQuestion(ctx context.Context, key string, ref api.Ref, full bool, name string, n int, apply func(*domain.Question, time.Time)) (api.Written[api.TicketView], error) {
	var target int
	find := func(t *domain.Ticket) (*domain.Round, error) {
		for i := range t.Rounds {
			for j, q := range t.Rounds[i].Questions {
				if q.Number == n {
					target = j
					return &t.Rounds[i], nil
				}
			}
		}
		return nil, notFound("there is no Question %d on %s", n, DisplayID(key, t.Number))
	}
	return c.onRound(ctx, key, ref, full, name, find, func(r *domain.Round, now time.Time) bool {
		apply(&r.Questions[target], now)
		return true
	})
}

// onRound runs one of the human's Round actions: only the human may take
// it, only on a Round that is not yet complete, and when it leaves no
// open Question the Ticket moves to Answered in the same write.
func (c *Core) onRound(ctx context.Context, key string, ref api.Ref, full bool, name string,
	find func(*domain.Ticket) (*domain.Round, error), apply func(*domain.Round, time.Time) bool) (api.Written[api.TicketView], error) {
	return c.updateTicket(key, name, ref, full, func(t *domain.Ticket) (bool, error) {
		if actor := api.ActorOf(ctx); actor != api.Human {
			return false, &api.Error{Code: api.ActionNotAllowed, AllowedActions: allowedActions(t.State, actor),
				Message: fmt.Sprintf("%s is a human action; this request is made as %s", name, actor)}
		}
		r, err := find(t)
		if err != nil {
			return false, err
		}
		if complete(*r) {
			return false, &api.Error{Code: api.ActionNotAllowed, AllowedActions: allowedActions(t.State, api.Human), Message: fmt.Sprintf(
				"Round %d on %s is complete, so its answers can't change; add a comment instead", r.Number, DisplayID(key, t.Number))}
		}
		now := c.now()
		if !apply(r, now) {
			return false, nil
		}
		if complete(*r) {
			if err := transition(ctx, t, key, "answered", "", now); err != nil {
				return false, err
			}
		}
		return true, nil
	})
}

// complete reports whether no Question in r is open.
func complete(r domain.Round) bool {
	for _, q := range r.Questions {
		if q.Status == domain.QuestionOpen {
			return false
		}
	}
	return true
}

// dismissOpen dismisses every open Question on t.
func dismissOpen(t *domain.Ticket, now time.Time) {
	for i := range t.Rounds {
		for j := range t.Rounds[i].Questions {
			if q := &t.Rounds[i].Questions[j]; q.Status == domain.QuestionOpen {
				q.Status, q.Closed = domain.QuestionDismissed, now
			}
		}
	}
}

func questionCount(t domain.Ticket) int {
	n := 0
	for _, r := range t.Rounds {
		n += len(r.Questions)
	}
	return n
}

func roundViews(rs []domain.Round) []api.RoundView {
	out := []api.RoundView{}
	for _, r := range rs {
		out = append(out, roundView(r))
	}
	return out
}

func roundView(r domain.Round) api.RoundView {
	v := api.RoundView{Number: r.Number, Claim: r.Claim, SessionID: r.SessionID, Posted: r.Posted, Questions: []api.QuestionView{}}
	for _, q := range r.Questions {
		qv := api.QuestionView{Number: q.Number, Title: q.Title, Body: q.Body, Recommended: q.Recommended, Status: q.Status, Answer: q.Answer}
		if !q.Closed.IsZero() {
			closed := q.Closed
			qv.Closed = &closed
		}
		v.Questions = append(v.Questions, qv)
	}
	return v
}
