package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// A Ticket's Changed Repositories (Spec code-central-848): the Factory
// reports them live while it holds the Ticket, and fails a finish its
// finish check finds wrong.
func init() {
	legacyRoutes["report_changed_repositories"] = "POST /api/p/{key}/tickets/{n}/report_changed_repositories"
	legacyRoutes["fail_finish"] = "POST /api/p/{key}/tickets/{n}/fail_finish"

	handlers["report_changed_repositories"] = func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.ReportChangedRepositoriesInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.ReportChangedRepositories(r.Context(), r.PathValue("key"), in)
	}
	handlers["fail_finish"] = func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.FailFinishInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.FailFinish(r.Context(), r.PathValue("key"), in)
	}
}
