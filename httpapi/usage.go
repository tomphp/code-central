package httpapi

import (
	"net/http"
	"strconv"

	"github.com/tomoram/code-central/core"
)

// UsagePath reads Usage (Spec code-central-201):
// GET /api/usage?project=<key>&days=<7|30|90>. Leaving out project reads
// every Project, Archived ones too; days defaults to 7. It is not one of
// core's operations, so it sits outside Routes.
const UsagePath = "GET /api/usage"

func usageRead(c *core.Core, r *http.Request) (any, error) {
	q := r.URL.Query()
	if err := onlyParams(q, "project", "days"); err != nil {
		return nil, err
	}
	in := core.UsageInput{Project: q.Get("project")}
	if d := q.Get("days"); d != "" {
		n, err := strconv.Atoi(d)
		if err != nil || n == 0 {
			return nil, &core.Error{Code: core.Invalid, Field: "days", Message: "days must be 7, 30 or 90, not " + strconv.Quote(d)}
		}
		in.Days = n
	}
	return c.Usage(r.Context(), in)
}
