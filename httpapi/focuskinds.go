package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Focus Kinds (Spec code-central-550): the human's choice of which Needs
// Me reasons are Focus Items for a Project.
func init() {
	Routes["set_focus_kinds"] = "POST /api/p/{key}/set_focus_kinds"

	handlers["set_focus_kinds"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SetFocusKindsInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SetFocusKinds(r.Context(), r.PathValue("key"), in)
	}
}
