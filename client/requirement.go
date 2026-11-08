package client

import (
	"context"
	"encoding/json"

	"github.com/tomoram/code-central/core"
)

// SubmitRequirement is submit_requirement: the Spec and each Stage are
// decoded as an acknowledging write's result.
func (c *Client) SubmitRequirement(ctx context.Context, key string, in core.SubmitRequirementInput) (core.SubmitRequirementResult, error) {
	var raw struct {
		Spec   json.RawMessage   `json:"spec"`
		Stages []json.RawMessage `json:"stages"`
	}
	if err := c.do(ctx, "POST", scoped(key, "submit_requirement"), in, &raw); err != nil {
		return core.SubmitRequirementResult{}, err
	}
	spec, err := decodeWritten[core.SpecView](raw.Spec, in.Full)
	if err != nil {
		return core.SubmitRequirementResult{}, err
	}
	out := core.SubmitRequirementResult{Spec: spec}
	for _, s := range raw.Stages {
		stage, err := decodeWritten[core.TicketView](s, in.Full)
		if err != nil {
			return core.SubmitRequirementResult{}, err
		}
		out.Stages = append(out.Stages, stage)
	}
	return out, nil
}
