package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Starting a Retro. It writes as human whatever Actor the request names;
// core sees to that.
func init() {
	Routes["submit_retro"] = "POST /api/p/{key}/submit_retro"

	handlers["submit_retro"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SubmitRetroInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SubmitRetro(r.Context(), r.PathValue("key"), in)
	}
}
