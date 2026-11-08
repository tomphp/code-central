package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
)

// GetStageGuide is get_stage_guide.
func (c *Client) GetStageGuide(ctx context.Context, in core.StageGuideInput) (core.StageGuideView, error) {
	var out core.StageGuideView
	return out, c.do(ctx, "GET", "/api/stage-guides/"+url.PathEscape(in.Stage), nil, &out)
}
