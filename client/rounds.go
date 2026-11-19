package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/ops"
)

// PostRound is post_round.
func (c *Client) PostRound(ctx context.Context, key string, in api.PostRoundInput) (api.PostRoundResult, error) {
	return call(ctx, c, ops.PostRound, key, in)
}

// PickUpAnswers is pick_up_answers.
func (c *Client) PickUpAnswers(ctx context.Context, key string, in api.PickUpAnswersInput) (api.PickUpAnswersResult, error) {
	return call(ctx, c, ops.PickUpAnswers, key, in)
}

// Answer is the human's answer; set Actor to human.
func (c *Client) Answer(ctx context.Context, key string, in api.AnswerInput) (api.Written[api.TicketView], error) {
	return written(ctx, c, ops.Answer, key, in, in.Full)
}

// AcceptRecommended is the human's accept_recommended; set Actor to human.
func (c *Client) AcceptRecommended(ctx context.Context, key string, in api.AcceptRecommendedInput) (api.Written[api.TicketView], error) {
	return written(ctx, c, ops.AcceptRecommended, key, in, in.Full)
}

// DismissQuestion is the human's dismiss_question; set Actor to human.
func (c *Client) DismissQuestion(ctx context.Context, key string, in api.DismissQuestionInput) (api.Written[api.TicketView], error) {
	return written(ctx, c, ops.DismissQuestion, key, in, in.Full)
}

// Wait is one long-poll: it holds for up to a minute and returns the
// Ticket's State, at once if it has already left from.
func (c *Client) Wait(ctx context.Context, key string, ref api.Ref, from domain.State) (api.WaitResult, error) {
	var out api.WaitResult
	path := item(key, "tickets", ref, "wait") + "?" + url.Values{"from": {string(from)}}.Encode()
	return out, c.do(ctx, "GET", path, nil, &out)
}
