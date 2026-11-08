package frontdoor_test

import (
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// linkedItems adds linked Spec acme-4 and, under it, linked Ticket acme-5,
// closed externally.
func linkedItems(t *testing.T, srv *codecentraltest.Server) {
	as := core.WithActor(t.Context(), core.Integration)
	must(t, func() error {
		_, err := srv.Core.CreateLinkedSpec(as, "acme", core.CreateLinkedSpecInput{Title: "Epic", Link: core.LinkInput{Integration: "github", ID: "1"}})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.CreateLinkedTicket(as, "acme", core.CreateLinkedTicketInput{Spec: "4", Title: "Issue", Link: core.LinkInput{Integration: "github", ID: "2"}})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.SetExternalState(as, "acme", core.SetExternalStateInput{Target: "5", ExternalState: store.ExternalClosed})
		return err
	})
}

// The Integration contract (ticket 42): HTTP only. Paths under
// /integration/ are called as the integration Actor (see httpDo).
func init() {
	for name, c := range map[string]opCase{
		"create_linked_spec": {
			ok:    call{http: httpCall{"POST", "/api/p/acme/integration/create_linked_spec", `{"title":"Epic","link":{"integration":"github","id":"1","url":"https://github.com/acme/web/issues/1"}}`}},
			check: acknowledgement("acme-4", "title", "link"),
			bad:   call{http: httpCall{"POST", "/api/p/acme/integration/create_linked_spec", `{"title":"Epic","link":{"integration":"github"}}`}},
			code:  core.Invalid,
		},
		"create_linked_ticket": {
			ok: call{http: httpCall{"POST", "/api/p/acme/integration/create_linked_ticket", `{"spec":"1","title":"Issue","state":"Ready","link":{"integration":"github","id":"2"}}`}},
			check: func(t *testing.T, out map[string]any) {
				field("state", "Ready")(t, out)
				field("id", "2")(t, out["link"].(map[string]any))
			},
			bad:  call{http: httpCall{"POST", "/api/p/acme/integration/create_linked_ticket", `{"spec":"1","title":"Issue","state":"Done","link":{"integration":"github","id":"2"}}`}},
			code: core.Invalid,
		},
		"update_linked_spec": {
			setup: linkedItems,
			ok:    call{http: httpCall{"POST", "/api/p/acme/integration/specs/4/update_linked_spec", `{"title":"Epic 2"}`}},
			check: acknowledgement("acme-4", "title", "edits"),
			bad:   call{http: httpCall{"POST", "/api/p/acme/integration/specs/1/update_linked_spec", `{"title":"Not linked"}`}},
			code:  core.ActionNotAllowed,
		},
		"update_linked_ticket": {
			setup: linkedItems,
			ok:    call{http: httpCall{"POST", "/api/p/acme/integration/tickets/5/update_linked_ticket", `{"spec":"acme-1"}`}},
			check: func(t *testing.T, out map[string]any) { field("id", "acme-1")(t, out["spec"].(map[string]any)) },
			bad:   call{http: httpCall{"POST", "/api/p/acme/integration/tickets/2/update_linked_ticket", `{"title":"Not linked"}`}},
			code:  core.ActionNotAllowed,
		},
		"set_external_state": {
			setup: linkedItems,
			ok:    call{http: httpCall{"POST", "/api/p/acme/integration/set_external_state", `{"target":"5","external_state":"open"}`}},
			check: func(t *testing.T, out map[string]any) {
				field("kind", "ticket")(t, out)
				field("external_state", "open")(t, out["link"].(map[string]any))
			},
			bad:  call{http: httpCall{"POST", "/api/p/acme/integration/set_external_state", `{"target":"5","external_state":"gone"}`}},
			code: core.Invalid,
		},
		"find_linked": {
			setup: linkedItems,
			ok:    call{http: httpCall{"GET", "/api/p/acme/integration/linked?integration=github&id=2", ""}},
			check: func(t *testing.T, out map[string]any) {
				rows, _ := out["rows"].([]any)
				if len(rows) != 1 {
					t.Fatalf("linked %v", out)
				}
				field("id", "acme-5")(t, rows[0].(map[string]any))
			},
			bad:  call{http: httpCall{"GET", "/api/p/acme/integration/linked", ""}},
			code: core.Invalid,
		},
		"unlink": {
			setup: linkedItems,
			ok:    call{http: httpCall{"POST", "/api/p/acme/unlink", `{"target":"acme-5"}`}},
			check: func(t *testing.T, out map[string]any) {
				if out["link"].(map[string]any)["detached"] != true {
					t.Fatalf("unlink %v", out)
				}
			},
			bad:  call{http: httpCall{"POST", "/api/p/acme/unlink", `{"target":"acme-2"}`}},
			code: core.Invalid,
		},
	} {
		cases[name] = c
	}
}
