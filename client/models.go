package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
)

// The Models section of Project Options and the Reported Models (Spec
// code-central-301).

// SetModels is the human's set_models; set Actor to human.
func (c *Client) SetModels(ctx context.Context, key string, in core.SetModelsInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/set_models", in, in.Full)
}

// ReportModels is the Factory's report_models.
func (c *Client) ReportModels(ctx context.Context, key string, in core.ReportModelsInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/report_models", in, in.Full)
}

// ReportHeartbeat is the Factory's report_heartbeat (Spec code-central-305).
func (c *Client) ReportHeartbeat(ctx context.Context, key string, in core.ReportHeartbeatInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/report_heartbeat", in, in.Full)
}

// ReportBase is the Factory's report_base (code-central-394).
func (c *Client) ReportBase(ctx context.Context, key string, in core.ReportBaseInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/report_base", in, in.Full)
}
