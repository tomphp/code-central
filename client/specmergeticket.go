package client

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Spec merge Tickets (code-central-542).

// OpenSpecMerge is the Factory's open_spec_merge.
func (c *Client) OpenSpecMerge(ctx context.Context, key string, in core.OpenSpecMergeInput) (core.OpenSpecMergeResult, error) {
	var out core.OpenSpecMergeResult
	return out, c.do(ctx, "POST", item(key, "specs", in.Spec, "open_spec_merge"), in, &out)
}
