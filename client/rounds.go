package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// PostRound is post_round.
func (c *Client) PostRound(ctx context.Context, key string, in core.PostRoundInput) (core.PostRoundResult, error) {
	var out core.PostRoundResult
	return out, c.do(ctx, "POST", item(key, "tickets", in.Ticket, "post_round"), in, &out)
}

// PickUpAnswers is pick_up_answers.
func (c *Client) PickUpAnswers(ctx context.Context, key string, in core.PickUpAnswersInput) (core.PickUpAnswersResult, error) {
	var out core.PickUpAnswersResult
	return out, c.do(ctx, "POST", item(key, "tickets", in.Ticket, "pick_up_answers"), in, &out)
}

// Answer is the human's answer; set Actor to human.
func (c *Client) Answer(ctx context.Context, key string, in core.AnswerInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "answer"), in, in.Full)
}

// AcceptRecommended is the human's accept_recommended; set Actor to human.
func (c *Client) AcceptRecommended(ctx context.Context, key string, in core.AcceptRecommendedInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "accept_recommended"), in, in.Full)
}

// DismissQuestion is the human's dismiss_question; set Actor to human.
func (c *Client) DismissQuestion(ctx context.Context, key string, in core.DismissQuestionInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "dismiss_question"), in, in.Full)
}

// Wait is one long-poll: it holds for up to a minute and returns the
// Ticket's State, at once if it has already left from.
func (c *Client) Wait(ctx context.Context, key string, ref core.Ref, from store.State) (core.WaitResult, error) {
	var out core.WaitResult
	path := item(key, "tickets", ref, "wait") + "?" + url.Values{"from": {string(from)}}.Encode()
	return out, c.do(ctx, "GET", path, nil, &out)
}
