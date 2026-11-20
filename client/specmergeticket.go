package client

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/ops"
)

// Spec merge Tickets (code-central-542).

// OpenSpecMerge is the Factory's open_spec_merge.
func (c *Client) OpenSpecMerge(ctx context.Context, key string, in api.OpenSpecMergeInput) (api.OpenSpecMergeResult, error) {
	return call(ctx, c, ops.OpenSpecMerge, key, in)
}
