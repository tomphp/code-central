package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
	"github.com/tomoram/code-central/store"
)

// PostRound is post_round.
func (c *Client) PostRound(ctx context.Context, key string, in core.PostRoundInput) (core.PostRoundResult, error) {
	return call(ctx, c, ops.PostRound, key, in)
}

// PickUpAnswers is pick_up_answers.
func (c *Client) PickUpAnswers(ctx context.Context, key string, in core.PickUpAnswersInput) (core.PickUpAnswersResult, error) {
	return call(ctx, c, ops.PickUpAnswers, key, in)
}

// Answer is the human's answer; set Actor to human.
func (c *Client) Answer(ctx context.Context, key string, in core.AnswerInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.Answer, key, in, in.Full)
}

// AcceptRecommended is the human's accept_recommended; set Actor to human.
func (c *Client) AcceptRecommended(ctx context.Context, key string, in core.AcceptRecommendedInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.AcceptRecommended, key, in, in.Full)
}

// DismissQuestion is the human's dismiss_question; set Actor to human.
func (c *Client) DismissQuestion(ctx context.Context, key string, in core.DismissQuestionInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.DismissQuestion, key, in, in.Full)
}

// Wait is one long-poll: it holds for up to a minute and returns the
// Ticket's State, at once if it has already left from.
func (c *Client) Wait(ctx context.Context, key string, ref core.Ref, from store.State) (core.WaitResult, error) {
	var out core.WaitResult
	path := item(key, "tickets", ref, "wait") + "?" + url.Values{"from": {string(from)}}.Encode()
	return out, c.do(ctx, "GET", path, nil, &out)
}
