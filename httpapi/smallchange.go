package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Submitting a Small Change. It writes as human whatever Actor the request
// names; core sees to that.
func init() {
	Routes["submit_small_change"] = "POST /api/p/{key}/submit_small_change"

	handlers["submit_small_change"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SubmitSmallChangeInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SubmitSmallChange(r.Context(), r.PathValue("key"), in)
	}
}
