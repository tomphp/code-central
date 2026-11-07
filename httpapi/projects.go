package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Project modes (ticket 41).
func init() {
	Routes["set_project_mode"] = "POST /api/projects/{key}/set_project_mode"

	handlers["set_project_mode"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SetProjectModeInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SetProjectMode(r.Context(), r.PathValue("key"), in)
	}
}
