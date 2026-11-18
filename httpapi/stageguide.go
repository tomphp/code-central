package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Stage Guides (ticket 73; code-central-441). A guide is the Project's
// own, so its route takes the Project key.
func init() {
	legacyRoutes["get_stage_guide"] = "GET /api/p/{key}/stage-guides/{stage}"

	handlers["get_stage_guide"] = func(c *core.Core, r *http.Request) (any, error) {
		if err := onlyParams(r.URL.Query()); err != nil {
			return nil, err
		}
		return c.GetStageGuide(r.Context(), r.PathValue("key"), core.StageGuideInput{Stage: r.PathValue("stage")})
	}
}
