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

// PostUsagePath is where a Factory posts its sessions' model responses'
// token usage (Spec code-central-201). Like Session Events it is not one
// of core's operations, and always answers 204, even for an unknown
// session or a body it can't read, so a Factory never fails on it.
const PostUsagePath = "POST /api/usage"

func postUsage(c *core.Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			SessionID string               `json:"session_id"`
			Responses []core.UsageResponse `json:"responses"`
		}
		if decodeBody(r, &in) == nil {
			c.PostUsage(r.Context(), in.SessionID, in.Responses)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
