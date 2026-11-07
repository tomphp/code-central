package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// The Integration contract (ticket 42). The integration Actor's
// operations sit under /api/p/{key}/integration/; the human's unlink
// names its target (a Spec or a Ticket) in the body, like add_labels.
func init() {
	Routes["create_linked_spec"] = "POST /api/p/{key}/integration/create_linked_spec"
	Routes["create_linked_ticket"] = "POST /api/p/{key}/integration/create_linked_ticket"
	Routes["update_linked_spec"] = "POST /api/p/{key}/integration/specs/{n}/update_linked_spec"
	Routes["update_linked_ticket"] = "POST /api/p/{key}/integration/tickets/{n}/update_linked_ticket"
	Routes["set_external_state"] = "POST /api/p/{key}/integration/set_external_state"
	Routes["find_linked"] = "GET /api/p/{key}/integration/linked"
	Routes["unlink"] = "POST /api/p/{key}/unlink"

	handlers["create_linked_spec"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.CreateLinkedSpecInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.CreateLinkedSpec(r.Context(), r.PathValue("key"), in)
	}
	handlers["create_linked_ticket"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.CreateLinkedTicketInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.CreateLinkedTicket(r.Context(), r.PathValue("key"), in)
	}
	handlers["update_linked_spec"] = func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.UpdateLinkedSpecInput) *core.Ref { return &in.Spec })
		if err != nil {
			return nil, err
		}
		return c.UpdateLinkedSpec(r.Context(), r.PathValue("key"), in)
	}
	handlers["update_linked_ticket"] = func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.UpdateLinkedTicketInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.UpdateLinkedTicket(r.Context(), r.PathValue("key"), in)
	}
	handlers["set_external_state"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SetExternalStateInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SetExternalState(r.Context(), r.PathValue("key"), in)
	}
	handlers["find_linked"] = func(c *core.Core, r *http.Request) (any, error) {
		q := r.URL.Query()
		if err := onlyParams(q, "integration", "id"); err != nil {
			return nil, err
		}
		return c.FindLinked(r.Context(), r.PathValue("key"), core.FindLinkedInput{Integration: q.Get("integration"), ID: q.Get("id")})
	}
	handlers["unlink"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.UnlinkInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.Unlink(r.Context(), r.PathValue("key"), in)
	}
}
