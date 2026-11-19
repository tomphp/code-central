package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/ops"
)

// CIOverviewPath reads the CI view (code-central-258):
// GET /api/ci?project=<key>&running=true. Leaving out project reads every
// active Project with CI Tracking on; running=true keeps only the runs
// that count as running towards a Ticket's CI result (code-central-276).
// Like Usage it is not one of core's operations, so it sits outside
// Routes.
const CIOverviewPath = "GET /api/ci"

func ciOverviewRead(s ops.Service, r *http.Request) (any, error) {
	q := r.URL.Query()
	if err := onlyParams(q, "project", "running"); err != nil {
		return nil, err
	}
	running, err := boolParam(q, "running")
	if err != nil {
		return nil, err
	}
	return s.CIOverview(r.Context(), api.CIOverviewInput{Project: q.Get("project"), Running: running != nil && *running})
}
