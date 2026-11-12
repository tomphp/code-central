package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
)

// GetStageGuide is get_stage_guide: the named Project's guide.
func (c *Client) GetStageGuide(ctx context.Context, key string, in core.StageGuideInput) (core.StageGuideView, error) {
	var out core.StageGuideView
	return out, c.do(ctx, "GET", "/api/p/"+url.PathEscape(key)+"/stage-guides/"+url.PathEscape(in.Stage), nil, &out)
}

// SetSubagentExploration is the human's set_subagent_exploration; set
// Actor to human.
func (c *Client) SetSubagentExploration(ctx context.Context, key string, in core.SetSubagentExplorationInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/set_subagent_exploration", in, in.Full)
}
