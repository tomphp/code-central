package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Spec merge Tickets (code-central-542): the Factory asks Code Central to
// open the Ticket that merges the base into a conflicting Spec Branch.
func init() {
	Routes["open_spec_merge"] = "POST /api/p/{key}/specs/{n}/open_spec_merge"

	handlers["open_spec_merge"] = func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.OpenSpecMergeInput) *core.Ref { return &in.Spec })
		if err != nil {
			return nil, err
		}
		return c.OpenSpecMerge(r.Context(), r.PathValue("key"), in)
	}
}
