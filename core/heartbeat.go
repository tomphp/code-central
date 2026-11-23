package core

import (
	"context"

	"github.com/tomoram/code-central/api"
)

// ReportHeartbeat is the Factory's report_heartbeat: the Factories area
// takes the Heartbeat, and the Project's view, before and after, is what
// the caller is acknowledged with.
func (c *Core) ReportHeartbeat(ctx context.Context, key string, in api.ReportHeartbeatInput) (api.Written[api.ProjectView], error) {
	before, after, now, err := c.factories.ReportHeartbeat(ctx, key, in, c.projectView)
	if err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	return acknowledgeProject(before, after, in.Full, now)
}
