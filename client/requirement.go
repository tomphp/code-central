package client

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// SubmitRequirement is submit_requirement.
func (c *Client) SubmitRequirement(ctx context.Context, key string, in core.SubmitRequirementInput) (core.SubmitRequirementResult, error) {
	var out core.SubmitRequirementResult
	return out, c.do(ctx, "POST", scoped(key, "submit_requirement"), in, &out)
}
