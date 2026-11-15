package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// The Factory's wake (Spec code-central-743).
func init() {
	Routes["next_factory_wake"] = "GET /api/p/{key}/factory/next-wake"

	// next_factory_wake holds for up to core.WaitTimeout, as wait does.
	handlers["next_factory_wake"] = func(c *core.Core, r *http.Request) (any, error) {
		q := r.URL.Query()
		if err := onlyParams(q, "factory", "host"); err != nil {
			return nil, err
		}
		return c.NextFactoryWake(r.Context(), r.PathValue("key"), core.NextFactoryWakeInput{Factory: q.Get("factory"), Host: q.Get("host")})
	}
}
