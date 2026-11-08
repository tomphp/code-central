package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Stage Guides (ticket 73). Like the workflow, a guide is the same
// for every Project, so its route takes no Project key.
func init() {
	Routes["get_stage_guide"] = "GET /api/stage-guides/{stage}"

	handlers["get_stage_guide"] = func(c *core.Core, r *http.Request) (any, error) {
		if err := onlyParams(r.URL.Query()); err != nil {
			return nil, err
		}
		return c.GetStageGuide(r.Context(), core.StageGuideInput{Stage: r.PathValue("stage")})
	}
}
