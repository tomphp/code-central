package client

import (
	"context"
	"encoding/json"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/ops"
)

// SubmitRequirement is submit_requirement: the Spec and each Stage are
// decoded as an acknowledging write's result.
func (c *Client) SubmitRequirement(ctx context.Context, key string, in api.SubmitRequirementInput) (api.SubmitRequirementResult, error) {
	var raw struct {
		Spec   json.RawMessage   `json:"spec"`
		Stages []json.RawMessage `json:"stages"`
	}
	if err := request(ctx, c, ops.SubmitRequirement, key, in, &raw); err != nil {
		return api.SubmitRequirementResult{}, err
	}
	spec, err := decodeWritten[api.SpecView](raw.Spec, in.Full)
	if err != nil {
		return api.SubmitRequirementResult{}, err
	}
	out := api.SubmitRequirementResult{Spec: spec}
	for _, s := range raw.Stages {
		stage, err := decodeWritten[api.TicketView](s, in.Full)
		if err != nil {
			return api.SubmitRequirementResult{}, err
		}
		out.Stages = append(out.Stages, stage)
	}
	return out, nil
}
