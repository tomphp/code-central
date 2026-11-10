package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// CI Tracking (Spec code-central-247): the human's CI settings and CI
// Rules, and the Factory's report of its Push remote.
func init() {
	Routes["set_ci_tracking"] = "POST /api/p/{key}/set_ci_tracking"
	Routes["report_push_remote"] = "POST /api/p/{key}/report_push_remote"
	Routes["add_ci_rule"] = "POST /api/p/{key}/add_ci_rule"
	Routes["edit_ci_rule"] = "POST /api/p/{key}/edit_ci_rule"
	Routes["remove_ci_rule"] = "POST /api/p/{key}/remove_ci_rule"

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
	handlers["add_ci_rule"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.CIRuleInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.AddCIRule(r.Context(), r.PathValue("key"), in)
	}
	handlers["edit_ci_rule"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.CIRuleInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.EditCIRule(r.Context(), r.PathValue("key"), in)
	}
	handlers["remove_ci_rule"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.RemoveCIRuleInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.RemoveCIRule(r.Context(), r.PathValue("key"), in)
	}
}
