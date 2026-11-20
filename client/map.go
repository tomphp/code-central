package client

import (
	"context"
	"encoding/json"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/ops"
)

// SubmitMap is submit_map: the Spec and Charting Ticket are decoded as an
// acknowledging write's result.
func (c *Client) SubmitMap(ctx context.Context, key string, in api.SubmitMapInput) (api.SubmitMapResult, error) {
	var raw struct {
		Spec     json.RawMessage `json:"spec"`
		Charting json.RawMessage `json:"charting"`
	}
	if err := request(ctx, c, ops.SubmitMap, key, in, &raw); err != nil {
		return api.SubmitMapResult{}, err
	}
	spec, err := decodeWritten[api.SpecView](raw.Spec, in.Full)
	if err != nil {
		return api.SubmitMapResult{}, err
	}
	charting, err := decodeWritten[api.TicketView](raw.Charting, in.Full)
	if err != nil {
		return api.SubmitMapResult{}, err
	}
	return api.SubmitMapResult{Spec: spec, Charting: charting}, nil
}
