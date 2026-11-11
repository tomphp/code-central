package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// A Factory's Heartbeat (Spec code-central-305).
func init() {
	Routes["report_heartbeat"] = "POST /api/p/{key}/report_heartbeat"

	handlers["report_heartbeat"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.ReportHeartbeatInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.ReportHeartbeat(r.Context(), r.PathValue("key"), in)
	}
}

// The base a Factory reports under Pull Request (code-central-394).
func init() {
	Routes["report_base"] = "POST /api/p/{key}/report_base"

	handlers["report_base"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.ReportBaseInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.ReportBase(r.Context(), r.PathValue("key"), in)
	}
}
