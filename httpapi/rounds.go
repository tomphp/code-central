package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
)

// WaitPath is the long-poll behind `code-central wait`. It is not a core
// operation, so it sits outside Routes.
const WaitPath = "GET /api/p/{key}/tickets/{n}/wait"

// wait holds for up to core.WaitTimeout and returns the Ticket's State,
// at once if it has already left ?from=.
func wait(c *core.Core, r *http.Request) (any, error) {
	q := r.URL.Query()
	if err := onlyParams(q, "from"); err != nil {
		return nil, err
	}
	return c.Wait(r.Context(), r.PathValue("key"), core.Ref(r.PathValue("n")), domain.State(q.Get("from")))
}
