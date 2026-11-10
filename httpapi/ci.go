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

// The CI Watcher's own (code-central-253): the read it polls and its
// status.
func init() {
	Routes["get_ci_watch"] = "GET /api/p/{key}/ci-watch"
	Routes["report_ci_watcher_status"] = "POST /api/p/{key}/report_ci_watcher_status"

	handlers["get_ci_watch"] = func(c *core.Core, r *http.Request) (any, error) {
		return c.GetCIWatch(r.Context(), r.PathValue("key"))
	}
	handlers["report_ci_watcher_status"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.ReportCIWatcherStatusInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.ReportCIWatcherStatus(r.Context(), r.PathValue("key"), in)
	}
}

// Tracking a pushed Ticket's CI Runs (code-central-256): the Factory's
// pushed commits and the CI Watcher's runs.
func init() {
	Routes["report_pushed_commit"] = "POST /api/p/{key}/tickets/{n}/report_pushed_commit"
	Routes["report_ci_runs"] = "POST /api/p/{key}/report_ci_runs"

	handlers["report_pushed_commit"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.ReportPushedCommitInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		in.Ticket = core.Ref(r.PathValue("n"))
		return c.ReportPushedCommit(r.Context(), r.PathValue("key"), in)
	}
	handlers["report_ci_runs"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.ReportCIRunsInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.ReportCIRuns(r.Context(), r.PathValue("key"), in)
	}
}

// CI failed's failure options (code-central-261): the human's.
func init() {
	Routes["land_anyway"] = "POST /api/p/{key}/tickets/{n}/land_anyway"

	handlers["land_anyway"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.TicketInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		in.Ticket = core.Ref(r.PathValue("n"))
		return c.LandAnyway(r.Context(), r.PathValue("key"), in)
	}
}

// CIOverviewPath reads the CI view (code-central-258):
// GET /api/ci?project=<key>. Leaving out project reads every active
// Project with CI Tracking on. Like Usage it is not one of core's
// operations, so it sits outside Routes.
const CIOverviewPath = "GET /api/ci"

func ciOverviewRead(c *core.Core, r *http.Request) (any, error) {
	q := r.URL.Query()
	if err := onlyParams(q, "project"); err != nil {
		return nil, err
	}
	return c.CIOverview(r.Context(), core.CIOverviewInput{Project: q.Get("project")})
}
