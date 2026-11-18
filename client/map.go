package client

import (
	"context"
	"encoding/json"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
)

// SubmitMap is submit_map: the Spec and Charting Ticket are decoded as an
// acknowledging write's result.
func (c *Client) SubmitMap(ctx context.Context, key string, in core.SubmitMapInput) (core.SubmitMapResult, error) {
	var raw struct {
		Spec     json.RawMessage `json:"spec"`
		Charting json.RawMessage `json:"charting"`
	}
	if err := request(ctx, c, ops.SubmitMap, key, in, &raw); err != nil {
		return core.SubmitMapResult{}, err
	}
	spec, err := decodeWritten[core.SpecView](raw.Spec, in.Full)
	if err != nil {
		return core.SubmitMapResult{}, err
	}
	charting, err := decodeWritten[core.TicketView](raw.Charting, in.Full)
	if err != nil {
		return core.SubmitMapResult{}, err
	}
	return core.SubmitMapResult{Spec: spec, Charting: charting}, nil
}
