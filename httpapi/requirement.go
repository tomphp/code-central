package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Submitting a Requirement. It writes as human whatever Actor the request
// names; core sees to that.
func init() {
	Routes["submit_requirement"] = "POST /api/p/{key}/submit_requirement"

	handlers["submit_requirement"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SubmitRequirementInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SubmitRequirement(r.Context(), r.PathValue("key"), in)
	}
}
