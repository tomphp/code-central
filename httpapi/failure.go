package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Failure Reports and giving up (ticket 30).
func init() {
	Routes["report_failure"] = "POST /api/p/{key}/tickets/{n}/report_failure"
	Routes["give_up"] = "POST /api/p/{key}/tickets/{n}/give_up"
	Routes["send_back"] = "POST /api/p/{key}/tickets/{n}/send_back"
	Routes["to_draft"] = "POST /api/p/{key}/tickets/{n}/to_draft"

	handlers["report_failure"] = func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.ReportFailureInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.ReportFailure(r.Context(), r.PathValue("key"), in)
	}
	handlers["give_up"] = func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.GiveUpInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.GiveUp(r.Context(), r.PathValue("key"), in)
	}
	handlers["send_back"] = func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.SendBackInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.SendBack(r.Context(), r.PathValue("key"), in)
	}
	handlers["to_draft"] = func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.TicketInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.ToDraft(r.Context(), r.PathValue("key"), in)
	}
}
