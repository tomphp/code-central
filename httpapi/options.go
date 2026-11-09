package httpapi

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/tomoram/code-central/core"
)

// Options, the human's global preferences (Spec code-central-160). They
// belong to no Project, so their routes sit outside /api/p/{key}.
// set_options takes a JSON object of the Options to change.
func init() {
	Routes["get_options"] = "GET /api/options"
	Routes["set_options"] = "POST /api/set_options"

	handlers["get_options"] = func(c *core.Core, r *http.Request) (any, error) {
		if err := onlyParams(r.URL.Query()); err != nil {
			return nil, err
		}
		return c.GetOptions(r.Context())
	}
	handlers["set_options"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SetOptionsInput
		if err := decodeBody(r, &in); err != nil {
			// An unknown key is an unknown Option: name the ones there are.
			if e, ok := err.(*core.Error); ok && e.Field != "" && !slices.Contains(core.OptionNames, e.Field) {
				e.Message += fmt.Sprintf("; %q is not an Option (the Options are: %s)", e.Field, strings.Join(core.OptionNames, ", "))
			}
			return nil, err
		}
		return c.SetOptions(r.Context(), in)
	}
}
