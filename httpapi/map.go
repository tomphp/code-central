package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Submitting a Map. It writes as human whatever Actor the request names;
// core sees to that.
func init() {
	Routes["submit_map"] = "POST /api/p/{key}/submit_map"

	handlers["submit_map"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SubmitMapInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SubmitMap(r.Context(), r.PathValue("key"), in)
	}
}
