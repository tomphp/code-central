package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Repositories (Spec code-central-848). The list is the Project's own and
// the human's alone, so its one door is HTTP under the Project key, which
// the CLI's repository command calls through the client.
func init() {
	Routes["add_repository"] = "POST /api/p/{key}/add_repository"
	Routes["edit_repository"] = "POST /api/p/{key}/edit_repository"
	Routes["remove_repository"] = "POST /api/p/{key}/remove_repository"
	Routes["set_landing_order"] = "POST /api/p/{key}/set_landing_order"

	handlers["add_repository"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.AddRepositoryInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.AddRepository(r.Context(), r.PathValue("key"), in)
	}
	handlers["edit_repository"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.EditRepositoryInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.EditRepository(r.Context(), r.PathValue("key"), in)
	}
	handlers["remove_repository"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.RemoveRepositoryInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.RemoveRepository(r.Context(), r.PathValue("key"), in)
	}
	handlers["set_landing_order"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SetLandingOrderInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SetLandingOrder(r.Context(), r.PathValue("key"), in)
	}
}
