package client

import (
	"context"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
)

// The Models section of Project Options and the Reported Models (Spec
// code-central-301).

// SetModels is the human's set_models; set Actor to human.
func (c *Client) SetModels(ctx context.Context, key string, in core.SetModelsInput) (core.Written[core.ProjectView], error) {
	return written(ctx, c, ops.SetModels, key, in, in.Full)
}

// ReportModels is the Factory's report_models.
func (c *Client) ReportModels(ctx context.Context, key string, in core.ReportModelsInput) (core.Written[core.ProjectView], error) {
	return written(ctx, c, ops.ReportModels, key, in, in.Full)
}

// ReportHeartbeat is the Factory's report_heartbeat (Spec code-central-305).
func (c *Client) ReportHeartbeat(ctx context.Context, key string, in core.ReportHeartbeatInput) (core.Written[core.ProjectView], error) {
	return written(ctx, c, ops.ReportHeartbeat, key, in, in.Full)
}

// ReportBase is the Factory's report_base (code-central-394).
func (c *Client) ReportBase(ctx context.Context, key string, in core.ReportBaseInput) (core.Written[core.ProjectView], error) {
	return written(ctx, c, ops.ReportBase, key, in, in.Full)
}

// The Model Choice (Spec code-central-301).

// GetModelChoice is the Factory's get_model_choice.
func (c *Client) GetModelChoice(ctx context.Context, key string, in core.GetModelChoiceInput) (core.ModelChoice, error) {
	return call(ctx, c, ops.GetModelChoice, key, in)
}

// ReportModelChoice is the Factory's report_model_choice.
func (c *Client) ReportModelChoice(ctx context.Context, key string, in core.ReportModelChoiceInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.ReportModelChoice, key, in, in.Full)
}

// The Factory's wake (Spec code-central-743).

// NextFactoryWake is the Factory's next_factory_wake: it holds for up to
// core.WaitTimeout until a write the Factory's poll acts on reaches the
// Project, and returns woken then, or empty when none does.
func (c *Client) NextFactoryWake(ctx context.Context, key string, in core.NextFactoryWakeInput) (core.FactoryWake, error) {
	return call(ctx, c, ops.NextFactoryWake, key, in)
}
