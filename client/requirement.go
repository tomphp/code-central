package client

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// SubmitRequirement is submit_requirement. Its Spec and Stages are
// decoded into their Records when full was asked for, else into their
// Acknowledgements.
func (c *Client) SubmitRequirement(ctx context.Context, key string, in core.SubmitRequirementInput) (core.SubmitRequirementResult, error) {
	var out core.SubmitRequirementResult
	if in.Full {
		var rec struct {
			Spec   core.SpecView     `json:"spec"`
			Stages []core.TicketView `json:"stages"`
		}
		if err := c.do(ctx, "POST", scoped(key, "submit_requirement"), in, &rec); err != nil {
			return out, err
		}
		out.Spec.Record = &rec.Spec
		for i := range rec.Stages {
			out.Stages = append(out.Stages, core.Written[core.TicketView]{Record: &rec.Stages[i]})
		}
		return out, nil
	}
	var ack struct {
		Spec   core.Acknowledgement   `json:"spec"`
		Stages []core.Acknowledgement `json:"stages"`
	}
	if err := c.do(ctx, "POST", scoped(key, "submit_requirement"), in, &ack); err != nil {
		return out, err
	}
	out.Spec.Acknowledgement = ack.Spec
	for _, a := range ack.Stages {
		out.Stages = append(out.Stages, core.Written[core.TicketView]{Acknowledgement: a})
	}
	return out, nil
}
