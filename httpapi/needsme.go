package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Needs Me, moving by hand and Projects (ticket 32).
func init() {
	Routes["needs_me"] = "GET /api/needs-me"
	Routes["list_projects"] = "GET /api/projects"
	Routes["move"] = "POST /api/p/{key}/tickets/{n}/move"
	Routes["rename_project"] = "POST /api/p/{key}/rename_project"
	Routes["set_landing_method"] = "POST /api/p/{key}/set_landing_method"

	handlers["needs_me"] = func(c *core.Core, r *http.Request) (any, error) {
		if err := onlyParams(r.URL.Query()); err != nil {
			return nil, err
		}
		return c.NeedsMe(r.Context())
	}
	handlers["list_projects"] = func(c *core.Core, r *http.Request) (any, error) {
		if err := onlyParams(r.URL.Query()); err != nil {
			return nil, err
		}
		return c.ListProjects(r.Context())
	}
	handlers["move"] = func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.MoveInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.Move(r.Context(), r.PathValue("key"), in)
	}
	handlers["rename_project"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.RenameProjectInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.RenameProject(r.Context(), r.PathValue("key"), in)
	}
	handlers["set_landing_method"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SetLandingMethodInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SetLandingMethod(r.Context(), r.PathValue("key"), in)
	}
}
