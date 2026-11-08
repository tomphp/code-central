package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// QuestionInput is one Question of a Round being posted.
type QuestionInput struct {
	Title       string `json:"title"`
	Body        string `json:"body"`
	Recommended string `json:"recommended,omitempty"`
}

// PostRoundInput is post_round's input.
type PostRoundInput struct {
	Ticket    Ref             `json:"ticket"`
	ClaimID   string          `json:"claim_id"`
	SessionID string          `json:"session_id,omitempty"`
	Questions []QuestionInput `json:"questions"`
}

// PostRoundResult is post_round's result: the Round number and the next step.
type PostRoundResult struct {
	Round    int    `json:"round"`
	NextStep string `json:"next_step"`
}

// PickUpAnswersInput is pick_up_answers' input.
type PickUpAnswersInput struct {
	Ticket  Ref    `json:"ticket"`
	ClaimID string `json:"claim_id"`
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
	Ticket   Ref    `json:"ticket"`
	Question int    `json:"question"`
	Text     string `json:"text"`
}

// DismissQuestionInput is the human's dismiss_question(question).
type DismissQuestionInput struct {
	Ticket   Ref `json:"ticket"`
	Question int `json:"question"`
}

// AcceptRecommendedInput is the human's accept_recommended(round).
type AcceptRecommendedInput struct {
	Ticket Ref `json:"ticket"`
	Round  int `json:"round"`
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
	Number      int                  `json:"number"`
	Title       string               `json:"title"`
	Body        string               `json:"body"`
	Recommended string               `json:"recommended,omitempty"`
	Status      store.QuestionStatus `json:"status"`
	Answer      string               `json:"answer,omitempty"`
	Closed      *time.Time           `json:"closed,omitempty"`
}

// postRoundNextStep is post_round's next step (its row in Workflow);
// <id> becomes the Ticket ID.
const postRoundNextStep = "End your turn now, after running `code-central wait <id>` in the background if you can run background commands."

// PostRound is post_round(ticket, claim_id, session_id?, questions): the
// holder posts a numbered Round in one write, moving In Progress to Needs
// Answer and keeping the Claim. The session ID defaults to the Claim's.
func (c *Core) PostRound(ctx context.Context, key string, in PostRoundInput) (PostRoundResult, error) {
	if len(in.Questions) == 0 {
		return PostRoundResult{}, invalid("questions", "a Round needs at least one Question")
	}
	for i, q := range in.Questions {
		if strings.TrimSpace(q.Title) == "" {
			return PostRoundResult{}, invalid(fmt.Sprintf("questions[%d].title", i), "every Question needs a title")
		}
		if strings.TrimSpace(q.Body) == "" {
			return PostRoundResult{}, invalid(fmt.Sprintf("questions[%d].body", i), "every Question needs a body")
		}
	}
	var res PostRoundResult
	_, err := c.updateTicket(key, "post_round", in.Ticket, func(t *store.Ticket) (bool, error) {
		claim, _ := liveClaim(*t)
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
		r := store.Round{Number: len(t.Rounds) + 1, Claim: claim.Number, SessionID: session, Posted: now}
		n := questionCount(*t)
		for _, q := range in.Questions {
			n++
			r.Questions = append(r.Questions, store.Question{Number: n, Title: q.Title, Body: q.Body, Recommended: q.Recommended, Status: store.QuestionOpen})
		}
		t.Rounds = append(t.Rounds, r)
		res = PostRoundResult{Round: r.Number, NextStep: strings.ReplaceAll(nextStep("post_round"), "<id>", DisplayID(key, t.Number))}
		return true, nil
	})
	return res, err
}

// PickUpAnswers is pick_up_answers(ticket, claim_id): the holder moves
// Answered to In Progress in one step and gets the latest Round back.
func (c *Core) PickUpAnswers(ctx context.Context, key string, in PickUpAnswersInput) (PickUpAnswersResult, error) {
	var res PickUpAnswersResult
	_, err := c.updateTicket(key, "pick_up_answers", in.Ticket, func(t *store.Ticket) (bool, error) {
		if err := transition(ctx, t, key, "pick_up_answers", in.ClaimID, c.now()); err != nil {
			return false, err
		}
		r := t.Rounds[len(t.Rounds)-1]
		res = PickUpAnswersResult{Ticket: DisplayID(key, t.Number), Round: roundView(r), Comments: []CommentView{}}
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
func (c *Core) Answer(ctx context.Context, key string, in AnswerInput) (TicketView, error) {
	if strings.TrimSpace(in.Text) == "" {
		return TicketView{}, invalid("text", "an answer needs text; to decline a Question, dismiss it")
	}
	return c.onQuestion(ctx, key, in.Ticket, "answer", in.Question, func(q *store.Question, now time.Time) {
		q.Status, q.Answer, q.Closed = store.QuestionAnswered, in.Text, now
	})
}

// DismissQuestion is the human's dismiss_question(question): one Question
// declined without moving the Ticket, unless it was the last open one.
func (c *Core) DismissQuestion(ctx context.Context, key string, in DismissQuestionInput) (TicketView, error) {
	return c.onQuestion(ctx, key, in.Ticket, "dismiss_question", in.Question, func(q *store.Question, now time.Time) {
		q.Status, q.Answer, q.Closed = store.QuestionDismissed, "", now
	})
}

// AcceptRecommended is the human's accept_recommended(round): every open
// Question with a recommendation is answered with it.
func (c *Core) AcceptRecommended(ctx context.Context, key string, in AcceptRecommendedInput) (TicketView, error) {
	find := func(t *store.Ticket) (*store.Round, error) {
		if in.Round < 1 || in.Round > len(t.Rounds) {
			return nil, notFound("there is no Round %d on %s", in.Round, DisplayID(key, t.Number))
		}
		return &t.Rounds[in.Round-1], nil
	}
	return c.onRound(ctx, key, in.Ticket, "accept_recommended", find, func(r *store.Round, now time.Time) bool {
		changed := false
		for i := range r.Questions {
			if q := &r.Questions[i]; q.Status == store.QuestionOpen && q.Recommended != "" {
				q.Status, q.Answer, q.Closed = store.QuestionAnswered, q.Recommended, now
				changed = true
			}
		}
		return changed
	})
}

// onQuestion runs a Round action on Question n of the Ticket.
func (c *Core) onQuestion(ctx context.Context, key string, ref Ref, name string, n int, apply func(*store.Question, time.Time)) (TicketView, error) {
	var target int
	find := func(t *store.Ticket) (*store.Round, error) {
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
	return c.onRound(ctx, key, ref, name, find, func(r *store.Round, now time.Time) bool {
		apply(&r.Questions[target], now)
		return true
	})
}

// onRound runs one of the human's Round actions: only the human may take
// it, only on a Round that is not yet complete, and when it leaves no
// open Question the Ticket moves to Answered in the same write.
func (c *Core) onRound(ctx context.Context, key string, ref Ref, name string,
	find func(*store.Ticket) (*store.Round, error), apply func(*store.Round, time.Time) bool) (TicketView, error) {
	return c.updateTicket(key, name, ref, func(t *store.Ticket) (bool, error) {
		if actor := ActorOf(ctx); actor != Human {
			return false, &Error{Code: ActionNotAllowed, AllowedActions: allowedActions(t.State, actor),
				Message: fmt.Sprintf("%s is a human action; this request is made as %s", name, actor)}
		}
		r, err := find(t)
		if err != nil {
			return false, err
		}
		if complete(*r) {
			return false, &Error{Code: ActionNotAllowed, AllowedActions: allowedActions(t.State, Human), Message: fmt.Sprintf(
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
func complete(r store.Round) bool {
	for _, q := range r.Questions {
		if q.Status == store.QuestionOpen {
			return false
		}
	}
	return true
}

// dismissOpen dismisses every open Question on t.
func dismissOpen(t *store.Ticket, now time.Time) {
	for i := range t.Rounds {
		for j := range t.Rounds[i].Questions {
			if q := &t.Rounds[i].Questions[j]; q.Status == store.QuestionOpen {
				q.Status, q.Closed = store.QuestionDismissed, now
			}
		}
	}
}

func questionCount(t store.Ticket) int {
	n := 0
	for _, r := range t.Rounds {
		n += len(r.Questions)
	}
	return n
}

func roundViews(rs []store.Round) []RoundView {
	out := []RoundView{}
	for _, r := range rs {
		out = append(out, roundView(r))
	}
	return out
}

func roundView(r store.Round) RoundView {
	v := RoundView{Number: r.Number, Claim: r.Claim, SessionID: r.SessionID, Posted: r.Posted, Questions: []QuestionView{}}
	for _, q := range r.Questions {
		qv := QuestionView{Number: q.Number, Title: q.Title, Body: q.Body, Recommended: q.Recommended, Status: q.Status, Answer: q.Answer}
		if !q.Closed.IsZero() {
			closed := q.Closed
			qv.Closed = &closed
		}
		v.Questions = append(v.Questions, qv)
	}
	return v
}
