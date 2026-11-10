package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
)

// CI Tracking (Spec code-central-247).

// SetCITracking is the human's set_ci_tracking; set Actor to human.
func (c *Client) SetCITracking(ctx context.Context, key string, in core.SetCITrackingInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/set_ci_tracking", in, in.Full)
}

// ReportPushRemote is the Factory's report_push_remote.
func (c *Client) ReportPushRemote(ctx context.Context, key string, in core.ReportPushRemoteInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/report_push_remote", in, in.Full)
}
