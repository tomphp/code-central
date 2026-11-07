package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Scratch Import (ticket 40). Import writes as agent whatever Actor the
// request names; core sees to that.
func init() {
	Routes["get_import_guide"] = "GET /api/p/{key}/import-guide"
	Routes["import"] = "POST /api/p/{key}/import"

	handlers["get_import_guide"] = func(c *core.Core, r *http.Request) (any, error) {
		if err := onlyParams(r.URL.Query()); err != nil {
			return nil, err
		}
		return c.GetImportGuide(r.Context(), r.PathValue("key"))
	}
	handlers["import"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.ImportInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.Import(r.Context(), r.PathValue("key"), in)
	}
}
