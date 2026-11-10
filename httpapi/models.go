package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// The Models section of Project Options (Spec code-central-301): the
// human's model and effort per kind of work, and the Reported Models.
func init() {
	Routes["set_models"] = "POST /api/p/{key}/set_models"

	handlers["set_models"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SetModelsInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SetModels(r.Context(), r.PathValue("key"), in)
	}

	// The Factory's report of what its claude accepts (code-central-311).
	Routes["report_models"] = "POST /api/p/{key}/report_models"

	handlers["report_models"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.ReportModelsInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.ReportModels(r.Context(), r.PathValue("key"), in)
	}
}
