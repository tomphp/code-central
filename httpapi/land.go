package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Landing Accepted work (ADR 0006).
func init() {
	Routes["land"] = "POST /api/p/{key}/tickets/{n}/land"

	handlers["land"] = func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.LandInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.Land(r.Context(), r.PathValue("key"), in)
	}
}
