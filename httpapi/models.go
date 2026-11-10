package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// The Models section of Project Options (Spec code-central-301): the
// human's model and effort per kind of work.
func init() {
	Routes["set_models"] = "POST /api/p/{key}/set_models"

	handlers["set_models"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SetModelsInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SetModels(r.Context(), r.PathValue("key"), in)
	}
}
