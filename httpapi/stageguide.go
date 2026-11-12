package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Stage Guides (ticket 73; code-central-441). A guide is the Project's
// own, so its route takes the Project key.
func init() {
	Routes["get_stage_guide"] = "GET /api/p/{key}/stage-guides/{stage}"
	Routes["set_subagent_exploration"] = "POST /api/p/{key}/set_subagent_exploration"

	handlers["get_stage_guide"] = func(c *core.Core, r *http.Request) (any, error) {
		if err := onlyParams(r.URL.Query()); err != nil {
			return nil, err
		}
		return c.GetStageGuide(r.Context(), r.PathValue("key"), core.StageGuideInput{Stage: r.PathValue("stage")})
	}
	handlers["set_subagent_exploration"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SetSubagentExplorationInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SetSubagentExploration(r.Context(), r.PathValue("key"), in)
	}
}
