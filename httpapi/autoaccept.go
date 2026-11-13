package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Auto-accept (code-central-536). The setting is the Project's own and
// the human's alone, so its one door is HTTP under the Project key.
func init() {
	Routes["set_auto_accept"] = "POST /api/p/{key}/set_auto_accept"

	handlers["set_auto_accept"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SetAutoAcceptInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SetAutoAccept(r.Context(), r.PathValue("key"), in)
	}
}
