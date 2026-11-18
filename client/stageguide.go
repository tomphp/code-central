package client

import (
	"context"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
)

// GetStageGuide is get_stage_guide: the named Project's guide.
func (c *Client) GetStageGuide(ctx context.Context, key string, in core.StageGuideInput) (core.StageGuideView, error) {
	return call(ctx, c, ops.GetStageGuide, key, in)
}

// SetSubagentExploration is the human's set_subagent_exploration; set
// Actor to human.
func (c *Client) SetSubagentExploration(ctx context.Context, key string, in core.SetSubagentExplorationInput) (core.Written[core.ProjectView], error) {
	return written(ctx, c, ops.SetSubagentExploration, key, in, in.Full)
}
