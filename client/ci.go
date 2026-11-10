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

// AddCIRule is the human's add_ci_rule; set Actor to human.
func (c *Client) AddCIRule(ctx context.Context, key string, in core.CIRuleInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/add_ci_rule", in, in.Full)
}

// EditCIRule is the human's edit_ci_rule; set Actor to human.
func (c *Client) EditCIRule(ctx context.Context, key string, in core.CIRuleInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/edit_ci_rule", in, in.Full)
}

// RemoveCIRule is the human's remove_ci_rule; set Actor to human.
func (c *Client) RemoveCIRule(ctx context.Context, key string, in core.RemoveCIRuleInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/remove_ci_rule", in, in.Full)
}
