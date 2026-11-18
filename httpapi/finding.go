package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Blockers, Replacements and finding work (ticket 28).
func init() {
	for name, route := range map[string]struct {
		pattern string
		h       handlerFunc
	}{
		"add_blocker": {"POST /api/p/{key}/tickets/{n}/add_blocker", func(c *core.Core, r *http.Request) (any, error) {
			in, err := pathBody(r, func(in *core.BlockerInput) *core.Ref { return &in.Ticket })
			if err != nil {
				return nil, err
			}
			return c.AddBlocker(r.Context(), r.PathValue("key"), in)
		}},
		"remove_blocker": {"POST /api/p/{key}/tickets/{n}/remove_blocker", func(c *core.Core, r *http.Request) (any, error) {
			in, err := pathBody(r, func(in *core.BlockerInput) *core.Ref { return &in.Ticket })
			if err != nil {
				return nil, err
			}
			return c.RemoveBlocker(r.Context(), r.PathValue("key"), in)
		}},
		"cancel": {"POST /api/p/{key}/tickets/{n}/cancel", func(c *core.Core, r *http.Request) (any, error) {
			in, err := pathBody(r, func(in *core.CancelInput) *core.Ref { return &in.Ticket })
			if err != nil {
				return nil, err
			}
			return c.Cancel(r.Context(), r.PathValue("key"), in)
		}},
	} {
		legacyRoutes[name] = route.pattern
		handlers[name] = route.h
	}
}
