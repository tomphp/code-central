package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Submitting a Prototype. It writes as human whatever Actor the request
// names; core sees to that.
func init() {
	Routes["submit_prototype"] = "POST /api/p/{key}/submit_prototype"

	handlers["submit_prototype"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SubmitPrototypeInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SubmitPrototype(r.Context(), r.PathValue("key"), in)
	}
}
