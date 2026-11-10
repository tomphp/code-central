package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// CI Tracking (Spec code-central-247): the human's CI settings and the
// Factory's report of its Push remote.
func init() {
	Routes["set_ci_tracking"] = "POST /api/p/{key}/set_ci_tracking"
	Routes["report_push_remote"] = "POST /api/p/{key}/report_push_remote"

	handlers["set_ci_tracking"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SetCITrackingInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SetCITracking(r.Context(), r.PathValue("key"), in)
	}
	handlers["report_push_remote"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.ReportPushRemoteInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.ReportPushRemote(r.Context(), r.PathValue("key"), in)
	}
}
